package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	wellogicLayerURL = "https://gisagoegle.state.mi.us/arcgis/rest/services/EGLE/DwOpenData/MapServer/3"
	wellogicItemURL  = "https://www.arcgis.com/sharing/rest/content/items/58c98df11b6a411c97b8aeb839a695ad"
	echoQueryURL     = "https://echodata.epa.gov/echo/echo_rest_services.get_facilities"
	echoPageURL      = "https://echodata.epa.gov/echo/echo_rest_services.get_qid"
	echoSectorURL    = "https://echo.epa.gov/system/files/PFASHandlingIndustrySectors-Apr2023-Pub.xlsx"
	maxAdjacentBytes = 16 << 20
)

type Envelope struct {
	MinLongitude float64
	MinLatitude  float64
	MaxLongitude float64
	MaxLatitude  float64
}

type WellogicWell struct {
	ID              string   `json:"id"`
	WellType        string   `json:"wellType,omitempty"`
	Status          string   `json:"status,omitempty"`
	StaticWaterFt   *float64 `json:"staticWaterFt,omitempty"`
	StaticWaterFlag string   `json:"staticWaterFlag,omitempty"`
	LocationMethod  string   `json:"locationMethod,omitempty"`
	WithinCounty    string   `json:"withinCounty,omitempty"`
	Latitude        float64  `json:"latitude"`
	Longitude       float64  `json:"longitude"`
	DistanceM       float64  `json:"distanceM,omitempty"`
}

type WellogicResult struct {
	Wells         []WellogicWell
	SourceVintage string
	FetchedAt     time.Time
}

type ECHOFacility struct {
	RegistryID string   `json:"registryId"`
	Name       string   `json:"name"`
	City       string   `json:"city,omitempty"`
	State      string   `json:"state,omitempty"`
	NAICS      []string `json:"naics"`
}

type ECHOResult struct {
	Facilities []ECHOFacility
	QueryRows  int
	Truncated  bool
	FetchedAt  time.Time
}

type SupplementalClient struct {
	http             *http.Client
	wellogicLayerURL string
	wellogicItemURL  string
	echoQueryURL     string
	echoPageURL      string
	now              func() time.Time
}

func NewSupplementalClient(httpClient *http.Client) *SupplementalClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	return &SupplementalClient{
		http: httpClient, wellogicLayerURL: wellogicLayerURL, wellogicItemURL: wellogicItemURL,
		echoQueryURL: echoQueryURL, echoPageURL: echoPageURL, now: time.Now,
	}
}

func (c *SupplementalClient) FetchWellogic(ctx context.Context, envelope Envelope) (WellogicResult, error) {
	result := WellogicResult{Wells: []WellogicWell{}, FetchedAt: c.now().UTC()}
	itemValues := url.Values{"f": {"json"}}
	var item struct {
		Modified int64 `json:"modified"`
	}
	if err := c.getJSON(ctx, c.wellogicItemURL, itemValues, &item); err == nil && item.Modified > 0 {
		result.SourceVintage = time.UnixMilli(item.Modified).UTC().Format(time.RFC3339)
	}

	const margin = 0.03
	geometry := fmt.Sprintf("%.7f,%.7f,%.7f,%.7f", envelope.MinLongitude-margin, envelope.MinLatitude-margin, envelope.MaxLongitude+margin, envelope.MaxLatitude+margin)
	values := url.Values{
		"f": {"json"}, "where": {"1=1"}, "geometry": {geometry},
		"geometryType": {"esriGeometryEnvelope"}, "inSR": {"4326"}, "outSR": {"4326"},
		"spatialRel": {"esriSpatialRelIntersects"}, "returnGeometry": {"true"},
		"outFields":         {"WELLID,WELL_TYPE,WEL_STATUS,SWL,SWL_FLAG,METHD_COLL,WITHIN_CO"},
		"resultRecordCount": {"2000"},
	}
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Features []struct {
			Attributes struct {
				ID              any      `json:"WELLID"`
				WellType        string   `json:"WELL_TYPE"`
				Status          string   `json:"WEL_STATUS"`
				StaticWater     *float64 `json:"SWL"`
				StaticWaterFlag string   `json:"SWL_FLAG"`
				LocationMethod  string   `json:"METHD_COLL"`
				WithinCounty    string   `json:"WITHIN_CO"`
			} `json:"attributes"`
			Geometry struct {
				Longitude float64 `json:"x"`
				Latitude  float64 `json:"y"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := c.getJSON(ctx, c.wellogicLayerURL+"/query", values, &payload); err != nil {
		return result, err
	}
	if payload.Error != nil {
		return result, errors.New("Michigan Wellogic rejected the spatial query")
	}
	for _, feature := range payload.Features {
		if feature.Geometry.Latitude == 0 || feature.Geometry.Longitude == 0 {
			continue
		}
		result.Wells = append(result.Wells, WellogicWell{
			ID: fmt.Sprint(feature.Attributes.ID), WellType: feature.Attributes.WellType,
			Status: feature.Attributes.Status, StaticWaterFt: feature.Attributes.StaticWater,
			StaticWaterFlag: feature.Attributes.StaticWaterFlag, LocationMethod: feature.Attributes.LocationMethod,
			WithinCounty: feature.Attributes.WithinCounty, Latitude: feature.Geometry.Latitude,
			Longitude: feature.Geometry.Longitude,
		})
	}
	return result, nil
}

func (c *SupplementalClient) FetchECHOPotentialSectors(ctx context.Context, latitude, longitude float64) (ECHOResult, error) {
	result := ECHOResult{Facilities: []ECHOFacility{}, FetchedAt: c.now().UTC()}
	values := url.Values{
		"p_lat":    {strconv.FormatFloat(latitude, 'f', 7, 64)},
		"p_long":   {strconv.FormatFloat(longitude, 'f', 7, 64)},
		"p_radius": {"5"}, "responseset": {"1000"}, "tablelist": {"Y"}, "output": {"JSON"},
	}
	first, err := c.fetchECHOPage(ctx, c.echoQueryURL, values)
	if err != nil {
		return result, err
	}
	result.QueryRows = first.QueryRows
	pages := []echoPage{first}
	for page := 2; page <= 5 && (page-1)*1000 < first.QueryRows; page++ {
		next, err := c.fetchECHOPage(ctx, c.echoPageURL, url.Values{
			"qid": {first.QueryID}, "pageno": {strconv.Itoa(page)}, "responseset": {"1000"}, "output": {"JSON"},
		})
		if err != nil {
			return result, err
		}
		pages = append(pages, next)
	}
	result.Truncated = first.QueryRows > 5000
	for _, page := range pages {
		for _, facility := range page.Facilities {
			codes := splitCodes(facility.NAICS)
			if !hasPotentialPFASSector(codes) {
				continue
			}
			result.Facilities = append(result.Facilities, ECHOFacility{
				RegistryID: facility.RegistryID, Name: facility.Name, City: facility.City,
				State: facility.State, NAICS: codes,
			})
		}
	}
	return result, nil
}

type echoPage struct {
	QueryID    string
	QueryRows  int
	Facilities []struct {
		Name       string `json:"FacName"`
		City       string `json:"FacCity"`
		State      string `json:"FacState"`
		RegistryID string `json:"RegistryID"`
		NAICS      string `json:"FacNAICSCodes"`
	}
}

func (c *SupplementalClient) fetchECHOPage(ctx context.Context, endpoint string, values url.Values) (echoPage, error) {
	var payload struct {
		Results struct {
			Message    string          `json:"Message"`
			QueryID    string          `json:"QueryID"`
			QueryRows  json.RawMessage `json:"QueryRows"`
			Facilities []struct {
				Name       string `json:"FacName"`
				City       string `json:"FacCity"`
				State      string `json:"FacState"`
				RegistryID string `json:"RegistryID"`
				NAICS      string `json:"FacNAICSCodes"`
			} `json:"Facilities"`
		} `json:"Results"`
	}
	if err := c.getJSON(ctx, endpoint, values, &payload); err != nil {
		return echoPage{}, err
	}
	if !strings.EqualFold(payload.Results.Message, "Success") || payload.Results.QueryID == "" {
		return echoPage{}, errors.New("EPA ECHO did not return a usable facility query")
	}
	rows, err := flexibleInt(payload.Results.QueryRows)
	if err != nil {
		return echoPage{}, errors.New("EPA ECHO returned an invalid result count")
	}
	return echoPage{QueryID: payload.Results.QueryID, QueryRows: rows, Facilities: payload.Results.Facilities}, nil
}

func (c *SupplementalClient) getJSON(ctx context.Context, endpoint string, values url.Values, target any) error {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse adjacent source URL: %w", err)
	}
	requestURL.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build adjacent source request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "pfas-load-control/0.1")
	response, err := c.http.Do(request)
	if err != nil {
		return errors.New("adjacent evidence source could not be reached")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("adjacent evidence source returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAdjacentBytes+1))
	if err != nil || len(body) > maxAdjacentBytes {
		return errors.New("adjacent evidence response could not be read safely")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("adjacent evidence source returned invalid JSON")
	}
	return nil
}

func flexibleInt(raw json.RawMessage) (int, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strconv.Atoi(text)
	}
	var number int
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, err
	}
	return number, nil
}

func splitCodes(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '|' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

var potentialPFASNAICS = map[string]struct{}{
	"211120": {}, "211130": {}, "212221": {}, "212230": {}, "212291": {}, "221320": {},
	"313110": {}, "313210": {}, "313220": {}, "313230": {}, "313240": {}, "313310": {},
	"313320": {}, "314110": {}, "314910": {}, "314999": {}, "316110": {}, "316998": {},
	"322121": {}, "322130": {}, "322219": {}, "322220": {}, "323111": {}, "323120": {},
	"324110": {}, "324191": {}, "325110": {}, "325120": {}, "325130": {}, "325180": {},
	"325193": {}, "325199": {}, "325211": {}, "325212": {}, "325220": {}, "325320": {},
	"325510": {}, "325611": {}, "325612": {}, "325613": {}, "325910": {}, "325992": {},
	"325998": {}, "326112": {}, "326113": {}, "326121": {}, "326130": {}, "326211": {},
	"327215": {}, "327310": {}, "331313": {}, "332812": {}, "332813": {}, "332999": {},
	"333249": {}, "333316": {}, "333318": {}, "334220": {}, "334310": {}, "334412": {},
	"334413": {}, "334418": {}, "334419": {}, "335931": {}, "335999": {}, "339112": {},
	"424690": {}, "424710": {}, "442291": {}, "488119": {}, "561740": {}, "562112": {},
	"562211": {}, "562212": {}, "562213": {}, "562219": {}, "811420": {}, "922160": {},
	"928110": {},
}

func hasPotentialPFASSector(codes []string) bool {
	for _, code := range codes {
		if _, ok := potentialPFASNAICS[code]; ok {
			return true
		}
	}
	return false
}
