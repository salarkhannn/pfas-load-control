package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("registry entry not found")
	ErrInvalid  = errors.New("invalid registry input")
	ErrExternal = errors.New("registry service unavailable")
)

type Service struct {
	pool   *pgxpool.Pool
	arcgis *ArcGISClient
	nass   *NASSClient
	http   *http.Client
}

func NewService(pool *pgxpool.Pool, nassAPIKey string) *Service {
	service := &Service{
		pool:   pool,
		arcgis: NewArcGISClient(nil),
		nass:   NewNASSClient(nil),
		http:   &http.Client{Timeout: 30 * time.Second},
	}
	if nassAPIKey != "" {
		service.nass.SetAPIKey(nassAPIKey)
	}
	return service
}

func (s *Service) resolveWorkspace(ctx context.Context, key string) (db.PfasWorkspace, error) {
	keyHash, err := workspace.Hash(key)
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("%w: workspace key is malformed", ErrInvalid)
	}
	queries := db.New(s.pool)
	record, err := queries.GetWorkspaceByHash(ctx, keyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return queries.UpsertWorkspace(ctx, db.UpsertWorkspaceParams{ID: uuid.New(), KeyHash: keyHash})
	}
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("load workspace: %w", err)
	}
	return record, nil
}

type RegistryEntry struct {
	ID        string          `json:"id"`
	EntryType string          `json:"entryType"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	Latitude  *float64        `json:"latitude,omitempty"`
	Longitude *float64        `json:"longitude,omitempty"`
	UpdatedAt string          `json:"updatedAt"`
	Rank      float64         `json:"rank,omitempty"`
	DistanceM float64         `json:"distanceM,omitempty"`
}

type RegistryCreateEntryInput struct {
	EntryType string                 `json:"entryType" enum:"PLANT,FIELD,CONTRACTOR"`
	Name      string                 `json:"name" minLength:"1" maxLength:"300"`
	Data      map[string]interface{} `json:"data" default:"{}"`
	Latitude  *float64               `json:"latitude,omitempty"`
	Longitude *float64               `json:"longitude,omitempty"`
}

type ArcGISResponse struct {
	Features []struct {
		Attributes map[string]interface{} `json:"attributes"`
	} `json:"features"`
}

type NASSResponse struct {
	Data []NASSData `json:"data"`
}

type NASSData struct {
	Year          int     `json:"year"`
	CommodityDesc string  `json:"commodity_desc"`
	CountyName    string  `json:"county_name"`
	StateName     string  `json:"state_name"`
	Value         float64 `json:"value"`
}

// ArcGIS client for MiEnviro

type ArcGISClient struct {
	baseURL string
	client  *http.Client
}

func NewArcGISClient(client *http.Client) *ArcGISClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ArcGISClient{
		baseURL: "https://gisagoegle.state.mi.us/arcgis/rest/services/EGLE/MiEnviro/FeatureServer",
		client:  client,
	}
}

func (c *ArcGISClient) SearchFeatures(ctx context.Context, layerID int, where string, outFields []string, resultOffset, resultRecordCount int) ([]map[string]interface{}, error) {
	fields := ""
	for i, f := range outFields {
		if i > 0 {
			fields += ","
		}
		fields += f
	}
	url := fmt.Sprintf("%s/%d/query?f=json&where=%s&outFields=%s&resultOffset=%d&resultRecordCount=%d&returnGeometry=false",
		c.baseURL, layerID, where, fields, resultOffset, resultRecordCount)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExternal, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrExternal, err)
	}
	var ar ArcGISResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrExternal, err)
	}
	result := make([]map[string]interface{}, 0, len(ar.Features))
	for _, f := range ar.Features {
		result = append(result, f.Attributes)
	}
	return result, nil
}

// USDA NASS client

type NASSClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewNASSClient(client *http.Client) *NASSClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &NASSClient{
		baseURL: "https://quickstats.nass.usda.gov/api/api_GET",
		client:  client,
	}
}

func (c *NASSClient) SetAPIKey(key string) {
	c.apiKey = key
}

func (c *NASSClient) GetCropData(ctx context.Context, state string, county string, commodity string, year int) ([]NASSData, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: USDA NASS API key not configured", ErrExternal)
	}
	url := fmt.Sprintf("%s?key=%s&stat_desc=MICHIGAN&county_name=%s&commodity_desc=%s&year=%d&format=JSON",
		c.baseURL, c.apiKey, county, commodity, year)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExternal, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrExternal, err)
	}
	var nr NASSResponse
	if err := json.Unmarshal(body, &nr); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrExternal, err)
	}
	return nr.Data, nil
}

func pgTS(t pgtype.Timestamptz) string {
	if t.Valid {
		return t.Time.Format(time.RFC3339)
	}
	return ""
}

func (s *Service) CreateEntry(ctx context.Context, workspaceKey string, input RegistryCreateEntryInput) (RegistryEntry, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return RegistryEntry{}, err
	}
	id := uuid.New()
	dataBytes := []byte(`{}`)
	if input.Data != nil {
		var err error
		dataBytes, err = json.Marshal(input.Data)
		if err != nil {
			return RegistryEntry{}, fmt.Errorf("%w: invalid data", ErrInvalid)
		}
	}
	q := db.New(s.pool)
	row, err := q.CreateRegistryEntry(ctx, db.CreateRegistryEntryParams{
		ID: id, WorkspaceID: ws.ID, EntryType: input.EntryType, Name: input.Name,
		Data: dataBytes, Latitude: input.Latitude, Longitude: input.Longitude,
	})
	if err != nil {
		return RegistryEntry{}, fmt.Errorf("create registry entry: %w", err)
	}
	return RegistryEntry{
		ID: row.ID.String(), EntryType: row.EntryType, Name: row.Name,
		Data: json.RawMessage(row.Data), Latitude: row.Latitude, Longitude: row.Longitude,
		UpdatedAt: pgTS(row.UpdatedAt),
	}, nil
}

func (s *Service) List(ctx context.Context, workspaceKey, entryType string) ([]RegistryEntry, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	q := db.New(s.pool)
	rows, err := q.ListRegistryEntries(ctx, db.ListRegistryEntriesParams{WorkspaceID: ws.ID, Column2: entryType})
	if err != nil {
		return nil, err
	}
	result := make([]RegistryEntry, 0, len(rows))
	for _, r := range rows {
		result = append(result, RegistryEntry{
			ID: r.ID.String(), EntryType: r.EntryType, Name: r.Name,
			Data: json.RawMessage(r.Data), Latitude: r.Latitude, Longitude: r.Longitude,
			UpdatedAt: pgTS(r.UpdatedAt),
		})
	}
	return result, nil
}

func (s *Service) Search(ctx context.Context, workspaceKey, query string) ([]RegistryEntry, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	q := db.New(s.pool)
	rows, err := q.SearchRegistryEntries(ctx, db.SearchRegistryEntriesParams{WorkspaceID: ws.ID, PlaintoTsquery: query})
	if err != nil {
		return nil, err
	}
	result := make([]RegistryEntry, 0, len(rows))
	for _, r := range rows {
		result = append(result, RegistryEntry{
			ID: r.ID.String(), EntryType: r.EntryType, Name: r.Name,
			Data: json.RawMessage(r.Data), Latitude: r.Latitude, Longitude: r.Longitude,
			UpdatedAt: pgTS(r.UpdatedAt), Rank: float64(r.Rank),
		})
	}
	return result, nil
}

func (s *Service) FindNearby(ctx context.Context, workspaceKey string, lat, lng float64, entryType string, limit int) ([]RegistryEntry, error) {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := db.New(s.pool)
	rows, err := q.FindNearbyRegistryEntries(ctx, db.FindNearbyRegistryEntriesParams{
		WorkspaceID: ws.ID, StMakepoint: lng, StMakepoint_2: lat, EntryType: entryType, Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	result := make([]RegistryEntry, 0, len(rows))
	for _, r := range rows {
		e := RegistryEntry{
			ID: r.ID.String(), EntryType: r.EntryType, Name: r.Name,
			Data: json.RawMessage(r.Data), Latitude: r.Latitude, Longitude: r.Longitude,
			UpdatedAt: pgTS(r.UpdatedAt),
		}
		if dist, ok := r.DistanceM.(float64); ok {
			e.DistanceM = dist
		}
		result = append(result, e)
	}
	return result, nil
}

func (s *Service) Delete(ctx context.Context, workspaceKey, entryID string) error {
	ws, err := s.resolveWorkspace(ctx, workspaceKey)
	if err != nil {
		return err
	}
	eID, err := uuid.Parse(entryID)
	if err != nil {
		return ErrInvalid
	}
	return db.New(s.pool).DeleteRegistryEntry(ctx, db.DeleteRegistryEntryParams{ID: eID, WorkspaceID: ws.ID})
}

// Match scoring

func (s *Service) ScoreMatch(plant, field RegistryEntry) (float64, []string) {
	score := 0.0
	reasons := []string{}

	if plant.Latitude != nil && plant.Longitude != nil && field.Latitude != nil && field.Longitude != nil {
		dist := haversineDistance(*plant.Latitude, *plant.Longitude, *field.Latitude, *field.Longitude)
		if dist < 50000 {
			score += 0.3
			reasons = append(reasons, fmt.Sprintf("within %dkm", int(dist/1000)))
		} else if dist < 100000 {
			score += 0.15
			reasons = append(reasons, fmt.Sprintf("within %dkm", int(dist/1000)))
		}
	}

	plantName := normalizeString(plant.Name)
	fieldName := normalizeString(field.Name)
	if len(plantName) > 0 && len(fieldName) > 0 {
		sim := stringSimilarity(plantName, fieldName)
		if sim > 0.5 {
			score += 0.2 * sim
			reasons = append(reasons, "name similarity")
		}
	}

	if plant.Data != nil && field.Data != nil {
		var plantData, fieldData map[string]interface{}
		if json.Unmarshal(plant.Data, &plantData) == nil && json.Unmarshal(field.Data, &fieldData) == nil {
			overlap := dataOverlap(plantData, fieldData)
			if overlap > 0 {
				score += 0.3 * overlap
				reasons = append(reasons, "data attribute overlap")
			}
		}
	}

	if score == 0 {
		score = 0.05
		reasons = append(reasons, "no strong signals")
	}

	return math.Min(score, 1.0), reasons
}

func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func normalizeString(s string) string {
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result = append(result, r)
		}
	}
	return string(result)
}

func stringSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[rune]bool)
	setB := make(map[rune]bool)
	for _, r := range a {
		setA[r] = true
	}
	for _, r := range b {
		setB[r] = true
	}
	intersect := 0
	for r := range setA {
		if setB[r] {
			intersect++
		}
	}
	union := len(setA) + len(setB) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

func dataOverlap(a, b map[string]interface{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	keys := make(map[string]bool)
	for k := range a {
		keys[k] = true
	}
	overlap := 0
	for k := range b {
		if keys[k] {
			overlap++
		}
	}
	union := len(keys)
	for k := range b {
		if !keys[k] {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(overlap) / float64(union)
}
