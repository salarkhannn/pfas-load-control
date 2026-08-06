package responseplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	landfillDatasetURL = "https://gis-egle.hub.arcgis.com/api/download/v1/items/43afb115983b4c62900c7ab129e0a3e0/geojson?layers=6"
	landfillItemURL    = "https://www.arcgis.com/sharing/rest/content/items/43afb115983b4c62900c7ab129e0a3e0?f=pjson"
	maxSourceBytes     = 8 << 20
)

type Landfill struct {
	WDSID              string
	Name               string
	FacilityType       string
	Address            string
	City               string
	County             string
	Latitude           float64
	Longitude          float64
	DisposalAreaStatus string
	SourceURL          string
	StraightlineKM     float64
}

type LandfillResult struct {
	Facilities    []Landfill
	SourceURL     string
	SourceVintage string
	FetchedAt     time.Time
	Raw           json.RawMessage
}

type LandfillSource interface {
	FetchActiveTypeII(context.Context, float64, float64, int) (LandfillResult, error)
}

type EGLandfillClient struct {
	http *http.Client
	now  func() time.Time
}

func NewEGLandfillClient(client *http.Client) *EGLandfillClient {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &EGLandfillClient{http: client, now: time.Now}
}

func (c *EGLandfillClient) FetchActiveTypeII(ctx context.Context, originLat, originLng float64, limit int) (LandfillResult, error) {
	if originLat < -90 || originLat > 90 || originLng < -180 || originLng > 180 {
		return LandfillResult{}, errors.New("facility origin is outside valid coordinate bounds")
	}
	if limit < 1 || limit > 5 {
		return LandfillResult{}, errors.New("landfill shortlist limit must be one to five")
	}
	result := LandfillResult{SourceURL: landfillDatasetURL, FetchedAt: c.now().UTC()}
	var item struct {
		Modified int64 `json:"modified"`
	}
	if err := c.getJSON(ctx, landfillItemURL, &item); err == nil && item.Modified > 0 {
		result.SourceVintage = time.UnixMilli(item.Modified).UTC().Format(time.RFC3339)
	}
	body, err := c.get(ctx, landfillDatasetURL)
	if err != nil {
		return result, err
	}
	result.Raw = append(json.RawMessage(nil), body...)
	facilities, err := parseLandfills(body, originLat, originLng)
	if err != nil {
		return result, err
	}
	result.Facilities = facilities
	sort.Slice(result.Facilities, func(i, j int) bool {
		if result.Facilities[i].StraightlineKM != result.Facilities[j].StraightlineKM {
			return result.Facilities[i].StraightlineKM < result.Facilities[j].StraightlineKM
		}
		return result.Facilities[i].WDSID < result.Facilities[j].WDSID
	})
	if len(result.Facilities) > limit {
		result.Facilities = result.Facilities[:limit]
	}
	return result, nil
}

func parseLandfills(body []byte, originLat, originLng float64) ([]Landfill, error) {
	var collection struct {
		Type     string `json:"type"`
		Features []struct {
			Properties struct {
				WDSID              any    `json:"wdsid"`
				LegalName          string `json:"legalsitename"`
				SpecificName       string `json:"specificsitename"`
				ActivityCode       string `json:"actcode"`
				FacilityType       string `json:"facilitytype"`
				Address            string `json:"addrline1"`
				City               string `json:"city"`
				County             string `json:"countyname"`
				Latitude           string `json:"latdeccord"`
				Longitude          string `json:"longdeccord"`
				DisposalAreaStatus string `json:"disposalareastatus"`
				LandfillLink       string `json:"landfilllink"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &collection); err != nil || collection.Type != "FeatureCollection" {
		return nil, errors.New("Michigan landfill inventory returned invalid GeoJSON") //lint:ignore ST1005 jurisdiction name is a proper noun
	}
	facilities := make([]Landfill, 0, len(collection.Features))
	for _, feature := range collection.Features {
		properties := feature.Properties
		if strings.TrimSpace(properties.ActivityCode) != "II" || !strings.EqualFold(strings.TrimSpace(properties.DisposalAreaStatus), "Active - Accepting") {
			continue
		}
		latitude, latErr := strconv.ParseFloat(strings.TrimSpace(properties.Latitude), 64)
		longitude, lngErr := strconv.ParseFloat(strings.TrimSpace(properties.Longitude), 64)
		if latErr != nil || lngErr != nil || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
			continue
		}
		name := strings.TrimSpace(properties.SpecificName)
		if name == "" {
			name = strings.TrimSpace(properties.LegalName)
		}
		if name == "" || strings.TrimSpace(properties.Address) == "" || strings.TrimSpace(properties.LandfillLink) == "" {
			continue
		}
		facilities = append(facilities, Landfill{
			WDSID: fmt.Sprint(properties.WDSID), Name: name,
			FacilityType: strings.TrimSpace(properties.FacilityType), Address: strings.TrimSpace(properties.Address),
			City: strings.TrimSpace(properties.City), County: strings.TrimSpace(properties.County),
			Latitude: latitude, Longitude: longitude, DisposalAreaStatus: strings.TrimSpace(properties.DisposalAreaStatus),
			SourceURL: strings.TrimSpace(properties.LandfillLink), StraightlineKM: haversineKM(originLat, originLng, latitude, longitude),
		})
	}
	return facilities, nil
}

func (c *EGLandfillClient) getJSON(ctx context.Context, endpoint string, target any) error {
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("authoritative source returned invalid JSON")
	}
	return nil
}

func (c *EGLandfillClient) get(ctx context.Context, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build authoritative source request: %w", err)
	}
	request.Header.Set("Accept", "application/geo+json, application/json")
	request.Header.Set("User-Agent", "pfas-load-control/0.1")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, errors.New("Michigan landfill inventory could not be reached") //lint:ignore ST1005 jurisdiction name is a proper noun
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Michigan landfill inventory returned HTTP %d", response.StatusCode) //lint:ignore ST1005 jurisdiction name is a proper noun
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSourceBytes+1))
	if err != nil || len(body) > maxSourceBytes {
		return nil, errors.New("Michigan landfill inventory could not be read safely") //lint:ignore ST1005 jurisdiction name is a proper noun
	}
	return body, nil
}

func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0088
	toRadians := math.Pi / 180
	dLat := (lat2 - lat1) * toRadians
	dLng := (lng2 - lng1) * toRadians
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*toRadians)*math.Cos(lat2*toRadians)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
