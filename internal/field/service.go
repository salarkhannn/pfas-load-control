package field

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
	"github.com/salarkhannn/pfas-load-control/internal/workspace"
)

var (
	ErrNotFound = errors.New("candidate field not found")
	ErrInvalid  = errors.New("invalid candidate field input")
	ErrConflict = errors.New("candidate field conflict")
	ErrExternal = errors.New("field location service unavailable")

	decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

type Service struct {
	pool   *pgxpool.Pool
	mireye *mireye.Client
}

func NewService(pool *pgxpool.Pool, mireyeClient *mireye.Client) *Service {
	return &Service{pool: pool, mireye: mireyeClient}
}

func (s *Service) Context(ctx context.Context, workspaceKey string, facilityID *string) (FieldContext, error) {
	workspaceRecord, err := s.workspace(ctx, workspaceKey)
	if errors.Is(err, ErrNotFound) {
		return FieldContext{Facilities: []FieldFacility{}, Fields: []Field{}}, nil
	}
	if err != nil {
		return FieldContext{}, err
	}
	queries := db.New(s.pool)
	facilities, err := queries.ListFacilitiesForWorkspace(ctx, workspaceRecord.ID)
	if err != nil {
		return FieldContext{}, fmt.Errorf("list field facilities: %w", err)
	}
	result := FieldContext{Facilities: make([]FieldFacility, 0, len(facilities))}
	for _, facility := range facilities {
		result.Facilities = append(result.Facilities, FieldFacility{
			ID: facility.ID.String(), Name: facility.Name, Jurisdiction: facility.Jurisdiction,
		})
	}
	result.Fields, err = s.list(ctx, queries, workspaceRecord.ID, facilityID, nil)
	if err != nil {
		return FieldContext{}, err
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, workspaceKey, fieldID string) (Field, error) {
	workspaceRecord, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return Field{}, err
	}
	fields, err := s.list(ctx, db.New(s.pool), workspaceRecord.ID, nil, &fieldID)
	if err != nil {
		return Field{}, err
	}
	if len(fields) != 1 {
		return Field{}, ErrNotFound
	}
	return fields[0], nil
}

func (s *Service) Create(ctx context.Context, workspaceKey, facilityID string, input CreateInput) (Field, error) {
	workspaceRecord, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return Field{}, err
	}
	facilityUUID, err := uuid.Parse(facilityID)
	if err != nil {
		return Field{}, ErrNotFound
	}
	input, locatorInput, geometry, geometryHash, err := validateCreateInput(input)
	if err != nil {
		return Field{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Field{}, fmt.Errorf("begin candidate field creation: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	if _, err := queries.GetFacilityForWorkspace(ctx, db.GetFacilityForWorkspaceParams{ID: facilityUUID, WorkspaceID: workspaceRecord.ID}); errors.Is(err, pgx.ErrNoRows) {
		return Field{}, ErrNotFound
	} else if err != nil {
		return Field{}, fmt.Errorf("load field facility: %w", err)
	}
	fieldID := uuid.New()
	record, err := queries.CreateCandidateField(ctx, db.CreateCandidateFieldParams{
		ID:             fieldID,
		WorkspaceID:    workspaceRecord.ID,
		FacilityID:     facilityUUID,
		Name:           input.Name,
		NormalizedName: normalize(input.Name),
		LocatorKind:    string(input.LocatorKind),
		LocatorInput:   locatorInput,
		Status:         string(StatusNeedsLocation),
	})
	if isUniqueViolation(err) {
		return Field{}, fmt.Errorf("%w: a field with this name already exists for the facility", ErrConflict)
	}
	if err != nil {
		return Field{}, fmt.Errorf("save candidate field: %w", err)
	}
	if len(geometry) > 0 {
		geometryID, err := s.saveGeometry(ctx, queries, record, geometry, geometryHash, "UPLOADED_GEOJSON", uuid.NullUUID{}, false)
		if err != nil {
			return Field{}, err
		}
		record.CurrentGeometryID = uuid.NullUUID{UUID: geometryID, Valid: true}
	}
	if err := syncFieldState(ctx, queries, record); err != nil {
		return Field{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Field{}, fmt.Errorf("commit candidate field creation: %w", err)
	}
	return s.Get(ctx, workspaceKey, fieldID.String())
}

func (s *Service) Resolve(ctx context.Context, workspaceKey, fieldID string) (Field, error) {
	workspaceRecord, fieldUUID, record, err := s.loadBase(ctx, workspaceKey, fieldID)
	if err != nil {
		return Field{}, err
	}
	if record.LocatorKind == string(LocatorGeoJSON) {
		return Field{}, fmt.Errorf("%w: an uploaded boundary does not need address resolution", ErrConflict)
	}
	kind := mapLookupKind(LocatorKind(record.LocatorKind))
	return s.resolveInput(ctx, workspaceKey, workspaceRecord.ID, fieldUUID, record.LocatorInput, kind)
}

func (s *Service) SelectCandidate(ctx context.Context, workspaceKey, fieldID string, candidateIndex int) (Field, error) {
	workspaceRecord, fieldUUID, _, err := s.loadBase(ctx, workspaceKey, fieldID)
	if err != nil {
		return Field{}, err
	}
	lookup, err := db.New(s.pool).GetLatestLocationLookup(ctx, fieldUUID)
	if errors.Is(err, pgx.ErrNoRows) || lookup.Disposition != "clarify" {
		return Field{}, fmt.Errorf("%w: this field has no unresolved location choices", ErrConflict)
	}
	if err != nil {
		return Field{}, fmt.Errorf("load location choices: %w", err)
	}
	var candidates []Candidate
	if err := json.Unmarshal(lookup.Candidates, &candidates); err != nil {
		return Field{}, fmt.Errorf("decode stored location choices: %w", err)
	}
	if candidateIndex < 0 || candidateIndex >= len(candidates) {
		return Field{}, fmt.Errorf("%w: select one of the listed location choices", ErrInvalid)
	}
	selected := candidates[candidateIndex]
	if selected.Latitude == nil || selected.Longitude == nil {
		return Field{}, fmt.Errorf("%w: the selected location has no usable coordinates", ErrConflict)
	}
	input := strconv.FormatFloat(*selected.Latitude, 'f', -1, 64) + "," + strconv.FormatFloat(*selected.Longitude, 'f', -1, 64)
	return s.resolveInput(ctx, workspaceKey, workspaceRecord.ID, fieldUUID, input, mireye.LookupCoordinate)
}

func (s *Service) ConfirmParcel(ctx context.Context, workspaceKey, fieldID string) (Field, error) {
	workspaceRecord, fieldUUID, _, err := s.loadBase(ctx, workspaceKey, fieldID)
	if err != nil {
		return Field{}, err
	}
	lookup, err := db.New(s.pool).GetLatestLocationLookup(ctx, fieldUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Field{}, fmt.Errorf("%w: resolve the field before confirming its parcel", ErrConflict)
	}
	if err != nil {
		return Field{}, fmt.Errorf("load parcel evidence: %w", err)
	}
	if lookup.Disposition != "resolved" || !isMichigan(lookup.State) || len(lookup.ParcelGeometry) == 0 {
		return Field{}, fmt.Errorf("%w: no safely resolved Michigan parcel is available to confirm", ErrConflict)
	}
	geometry, geometryHash, err := canonicalGeometry(string(lookup.ParcelGeometry))
	if err != nil {
		return Field{}, fmt.Errorf("%w: Mireye parcel boundary is invalid: %v", ErrConflict, err)
	}
	if err := s.attachGeometry(ctx, workspaceRecord.ID, fieldUUID, geometry, geometryHash, "MIREYE_PARCEL", uuid.NullUUID{UUID: lookup.ID, Valid: true}, true); err != nil {
		return Field{}, err
	}
	return s.Get(ctx, workspaceKey, fieldID)
}

func (s *Service) SetGeometry(ctx context.Context, workspaceKey, fieldID, geoJSON string) (Field, error) {
	workspaceRecord, fieldUUID, _, err := s.loadBase(ctx, workspaceKey, fieldID)
	if err != nil {
		return Field{}, err
	}
	geometry, geometryHash, err := canonicalGeometry(geoJSON)
	if err != nil {
		return Field{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := s.attachGeometry(ctx, workspaceRecord.ID, fieldUUID, geometry, geometryHash, "UPLOADED_GEOJSON", uuid.NullUUID{}, false); err != nil {
		return Field{}, err
	}
	return s.Get(ctx, workspaceKey, fieldID)
}

func (s *Service) ConfirmGeometry(ctx context.Context, workspaceKey, fieldID string) (Field, error) {
	workspaceRecord, fieldUUID, _, err := s.loadBase(ctx, workspaceKey, fieldID)
	if err != nil {
		return Field{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Field{}, fmt.Errorf("begin field boundary confirmation: %w", err)
	}
	defer tx.Rollback(ctx)
	txQueries := db.New(tx)
	record, err := txQueries.GetCandidateFieldForUpdate(ctx, db.GetCandidateFieldForUpdateParams{
		ID: fieldUUID, WorkspaceID: workspaceRecord.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Field{}, ErrNotFound
	}
	if err != nil {
		return Field{}, fmt.Errorf("lock candidate field: %w", err)
	}
	if !record.CurrentGeometryID.Valid {
		return Field{}, fmt.Errorf("%w: upload a boundary before confirming it", ErrConflict)
	}
	confirmedAt, err := txQueries.GetCurrentFieldGeometryConfirmation(ctx, db.GetCurrentFieldGeometryConfirmationParams{
		ID: record.CurrentGeometryID.UUID, FieldID: fieldUUID,
	})
	if err != nil {
		return Field{}, fmt.Errorf("load current field boundary: %w", err)
	}
	if !confirmedAt.Valid {
		updated, err := txQueries.ConfirmCurrentUploadedGeometry(ctx, db.ConfirmCurrentUploadedGeometryParams{
			FieldID: fieldUUID, WorkspaceID: workspaceRecord.ID,
		})
		if err != nil {
			return Field{}, fmt.Errorf("confirm uploaded field boundary: %w", err)
		}
		if updated != 1 {
			return Field{}, fmt.Errorf("%w: only an uploaded boundary awaiting confirmation can be confirmed", ErrConflict)
		}
	}
	if err := syncFieldState(ctx, txQueries, record); err != nil {
		return Field{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Field{}, fmt.Errorf("commit field boundary confirmation: %w", err)
	}
	return s.Get(ctx, workspaceKey, fieldID)
}

func (s *Service) UpdateDetails(ctx context.Context, workspaceKey, fieldID string, input DetailsInput) (Field, error) {
	workspaceRecord, fieldUUID, _, err := s.loadBase(ctx, workspaceKey, fieldID)
	if err != nil {
		return Field{}, err
	}
	cleanDetails(&input)
	usableAcres, err := decimal(input.UsableAcres, false)
	if err != nil {
		return Field{}, fmt.Errorf("%w: usable acres must be a positive decimal", ErrInvalid)
	}
	agronomicRate, err := decimal(input.AgronomicRateDryTonsPerAcre, false)
	if err != nil {
		return Field{}, fmt.Errorf("%w: agronomic rate must be a positive decimal", ErrInvalid)
	}
	priorLoading, err := decimal(input.PriorLoadingDryTons, true)
	if err != nil {
		return Field{}, fmt.Errorf("%w: prior loading must be zero or a positive decimal", ErrInvalid)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Field{}, fmt.Errorf("begin field details update: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	record, err := queries.GetCandidateFieldForUpdate(ctx, db.GetCandidateFieldForUpdateParams{ID: fieldUUID, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Field{}, ErrNotFound
	}
	if err != nil {
		return Field{}, fmt.Errorf("lock candidate field: %w", err)
	}
	updated, err := queries.UpdateCandidateFieldDetails(ctx, db.UpdateCandidateFieldDetailsParams{
		ID:                       fieldUUID,
		WorkspaceID:              workspaceRecord.ID,
		MienviroSiteID:           input.MiEnviroSiteID,
		RmpApproved:              input.RMPApproved,
		RmpDocumentReference:     input.RMPDocumentReference,
		UsableAcres:              usableAcres,
		CropOrUse:                input.CropOrUse,
		AgronomicRateDryTonsAcre: agronomicRate,
		PriorLoadingDryTons:      priorLoading,
		KnownConstraints:         input.KnownConstraints,
		AccessConstraints:        input.AccessConstraints,
	})
	if err != nil {
		return Field{}, fmt.Errorf("update field details: %w", err)
	}
	if updated != 1 {
		return Field{}, ErrNotFound
	}
	record.MienviroSiteID = input.MiEnviroSiteID
	record.RmpApproved = input.RMPApproved
	record.UsableAcres = usableAcres
	record.AgronomicRateDryTonsAcre = agronomicRate
	record.PriorLoadingDryTons = priorLoading
	if err := syncFieldState(ctx, queries, record); err != nil {
		return Field{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Field{}, fmt.Errorf("commit field details: %w", err)
	}
	return s.Get(ctx, workspaceKey, fieldID)
}

func (s *Service) resolveInput(ctx context.Context, workspaceKey string, workspaceID, fieldID uuid.UUID, input string, kind mireye.LookupKind) (Field, error) {
	request := mireye.LookupRequest{Input: input, Kind: kind}
	requestHash, err := mireye.LookupRequestHash(request)
	if err != nil {
		return Field{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	queries := db.New(s.pool)
	if _, err := queries.GetLocationLookupByRequestHash(ctx, db.GetLocationLookupByRequestHashParams{FieldID: fieldID, RequestHash: requestHash}); err == nil {
		return s.Get(ctx, workspaceKey, fieldID.String())
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Field{}, fmt.Errorf("check prior location lookup: %w", err)
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result, err := s.mireye.Lookup(lookupCtx, request)
	if err != nil {
		return Field{}, fmt.Errorf("%w: %v", ErrExternal, err)
	}
	candidates := make([]Candidate, 0, len(result.Response.Candidates))
	for _, candidate := range result.Response.Candidates {
		candidates = append(candidates, Candidate{
			Label: candidate.Label, ResolvedAddress: candidate.ResolvedAddress,
			Latitude: candidate.Latitude, Longitude: candidate.Longitude,
			Confidence: candidate.Confidence, MatchMethod: candidate.MatchMethod,
		})
	}
	candidateJSON, err := json.Marshal(candidates)
	if err != nil {
		return Field{}, fmt.Errorf("encode location choices: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Field{}, fmt.Errorf("begin location resolution: %w", err)
	}
	defer tx.Rollback(ctx)
	txQueries := db.New(tx)
	record, err := txQueries.GetCandidateFieldForUpdate(ctx, db.GetCandidateFieldForUpdateParams{ID: fieldID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Field{}, ErrNotFound
	}
	if err != nil {
		return Field{}, fmt.Errorf("lock candidate field: %w", err)
	}
	parcelGeometry := []byte(nil)
	var parcelID, parcelMatchType, parcelSource *string
	var parcelDistance pgtype.Numeric
	if result.Response.Parcel != nil {
		parcelGeometry = result.Response.Parcel.Geometry
		parcelID = optional(result.Response.Parcel.ID)
		if parcelID == nil {
			parcelID = optional(result.Response.Parcel.APN)
		}
		parcelMatchType = optional(result.Response.Parcel.MatchType)
		parcelSource = optional(result.Response.Parcel.Source)
		parcelDistance = floatNumeric(result.Response.Parcel.MatchDistanceM)
	}
	_, err = txQueries.CreateLocationLookup(ctx, db.CreateLocationLookupParams{
		ID:                   uuid.New(),
		FieldID:              fieldID,
		WorkspaceID:          workspaceID,
		Input:                strings.TrimSpace(input),
		InputKind:            string(kind),
		RequestHash:          result.RequestHash,
		ResponseHash:         result.ResponseHash,
		RequestID:            optional(result.RequestID),
		SourceUrl:            result.SourceURL,
		Disposition:          result.Response.Disposition,
		Latitude:             floatNumeric(result.Response.Latitude),
		Longitude:            floatNumeric(result.Response.Longitude),
		ResolvedAddress:      optional(result.Response.ResolvedAddress),
		State:                optional(result.Response.State),
		County:               optional(result.Response.County),
		Fips:                 optional(result.Response.FIPS),
		Confidence:           floatNumeric(result.Response.Confidence),
		MatchMethod:          optional(result.Response.MatchMethod),
		ParcelID:             parcelID,
		ParcelGeometry:       parcelGeometry,
		ParcelMatchType:      parcelMatchType,
		ParcelMatchDistanceM: parcelDistance,
		ParcelSource:         parcelSource,
		ParcelUnavailable:    result.Response.ParcelUnavailable,
		Candidates:           candidateJSON,
		Reason:               optional(result.Response.Reason),
		Hint:                 optional(result.Response.Hint),
		Evidence:             result.Evidence,
		FetchedAt:            pgtype.Timestamptz{Time: result.FetchedAt, Valid: true},
	})
	if isUniqueViolation(err) {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return Field{}, fmt.Errorf("close duplicate location resolution: %w", rollbackErr)
		}
		return s.Get(ctx, workspaceKey, fieldID.String())
	}
	if err != nil {
		return Field{}, fmt.Errorf("store location evidence: %w", err)
	}
	if err := syncFieldState(ctx, txQueries, record); err != nil {
		return Field{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Field{}, fmt.Errorf("commit location resolution: %w", err)
	}
	return s.Get(ctx, workspaceKey, fieldID.String())
}

func (s *Service) attachGeometry(ctx context.Context, workspaceID, fieldID uuid.UUID, geometry json.RawMessage, geometryHash, source string, sourceLookupID uuid.NullUUID, confirmed bool) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin field boundary update: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	record, err := queries.GetCandidateFieldForUpdate(ctx, db.GetCandidateFieldForUpdateParams{ID: fieldID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock candidate field: %w", err)
	}
	geometryID, err := s.saveGeometry(ctx, queries, record, geometry, geometryHash, source, sourceLookupID, confirmed)
	if err != nil {
		return err
	}
	record.CurrentGeometryID = uuid.NullUUID{UUID: geometryID, Valid: true}
	if err := syncFieldState(ctx, queries, record); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit field boundary: %w", err)
	}
	return nil
}

func (s *Service) saveGeometry(ctx context.Context, queries *db.Queries, record db.PfasCandidateField, geometry json.RawMessage, geometryHash, source string, sourceLookupID uuid.NullUUID, confirmed bool) (uuid.UUID, error) {
	if existing, err := queries.GetFieldGeometryByHash(ctx, db.GetFieldGeometryByHashParams{FieldID: record.ID, GeometryHash: geometryHash}); err == nil {
		if err := queries.SetCandidateFieldGeometry(ctx, db.SetCandidateFieldGeometryParams{ID: record.ID, CurrentGeometryID: uuid.NullUUID{UUID: existing.ID, Valid: true}, WorkspaceID: record.WorkspaceID}); err != nil {
			return uuid.Nil, fmt.Errorf("restore field boundary version: %w", err)
		}
		if confirmed && !existing.ConfirmedAt.Valid {
			updated, err := queries.ConfirmFieldGeometryVersion(ctx, db.ConfirmFieldGeometryVersionParams{ID: existing.ID, FieldID: record.ID, WorkspaceID: record.WorkspaceID})
			if err != nil {
				return uuid.Nil, fmt.Errorf("confirm existing field boundary: %w", err)
			}
			if updated != 1 {
				return uuid.Nil, fmt.Errorf("confirm existing field boundary: geometry version not found")
			}
		}
		return existing.ID, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("check field boundary version: %w", err)
	}
	validation, err := queries.ValidateFieldGeometry(ctx, string(geometry))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: field boundary could not be validated", ErrInvalid)
	}
	if !validation.IsValid || !validation.HasArea || validation.GeometryType != "MULTIPOLYGON" {
		return uuid.Nil, fmt.Errorf("%w: field boundary is invalid: %s", ErrInvalid, validation.Reason)
	}
	version, err := queries.NextFieldGeometryVersion(ctx, record.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("number field boundary version: %w", err)
	}
	geometryID := uuid.New()
	confirmedAt := pgtype.Timestamptz{}
	if confirmed {
		confirmedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}
	if _, err := queries.CreateFieldGeometryVersion(ctx, db.CreateFieldGeometryVersionParams{
		ID: geometryID, FieldID: record.ID, WorkspaceID: record.WorkspaceID,
		Version: version, Source: source, SourceLookupID: sourceLookupID,
		StGeomfromgeojson: string(geometry), GeometryHash: geometryHash, ConfirmedAt: confirmedAt,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("save field boundary version: %w", err)
	}
	if err := queries.SetCandidateFieldGeometry(ctx, db.SetCandidateFieldGeometryParams{ID: record.ID, CurrentGeometryID: uuid.NullUUID{UUID: geometryID, Valid: true}, WorkspaceID: record.WorkspaceID}); err != nil {
		return uuid.Nil, fmt.Errorf("set current field boundary: %w", err)
	}
	return geometryID, nil
}

func syncFieldState(ctx context.Context, queries *db.Queries, record db.PfasCandidateField) error {
	lookup, lookupErr := queries.GetLatestLocationLookup(ctx, record.ID)
	hasLookup := lookupErr == nil
	if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
		return fmt.Errorf("load current location state: %w", lookupErr)
	}
	hasGeometry := record.CurrentGeometryID.Valid
	hasConfirmedGeometry := false
	if hasGeometry {
		confirmedAt, err := queries.GetCurrentFieldGeometryConfirmation(ctx, db.GetCurrentFieldGeometryConfirmationParams{
			ID: record.CurrentGeometryID.UUID, FieldID: record.ID,
		})
		if err != nil {
			return fmt.Errorf("load field boundary confirmation: %w", err)
		}
		hasConfirmedGeometry = confirmedAt.Valid
	}
	open := map[string]bool{}
	if !hasGeometry {
		switch {
		case !hasLookup || lookup.Disposition == "no_match":
			open["LOCATION_UNRESOLVED"] = true
		case lookup.Disposition == "clarify":
			open["LOCATION_AMBIGUOUS"] = true
		case lookup.Disposition == "resolved" && !isMichigan(lookup.State):
			open["OUTSIDE_MICHIGAN"] = true
		}
	}
	if !hasConfirmedGeometry {
		open["GEOMETRY_UNCONFIRMED"] = true
	}
	if record.RmpApproved == nil || !*record.RmpApproved {
		open["RMP_APPROVAL_MISSING"] = true
	}
	if !record.UsableAcres.Valid {
		open["USABLE_ACRES_MISSING"] = true
	}
	if !record.AgronomicRateDryTonsAcre.Valid {
		open["AGRONOMIC_RATE_MISSING"] = true
	}
	if !record.PriorLoadingDryTons.Valid {
		open["PRIOR_LOADING_MISSING"] = true
	}
	definitions := gapDefinitions()
	for code, definition := range definitions {
		if open[code] {
			if err := queries.OpenFieldGap(ctx, db.OpenFieldGapParams{ID: uuid.New(), FieldID: record.ID, Code: code, Detail: definition.detail, Resolution: definition.resolution}); err != nil {
				return fmt.Errorf("open field gap %s: %w", code, err)
			}
		} else if err := queries.ResolveFieldGap(ctx, db.ResolveFieldGapParams{FieldID: record.ID, Code: code}); err != nil {
			return fmt.Errorf("resolve field gap %s: %w", code, err)
		}
	}
	status := StatusReady
	if open["LOCATION_UNRESOLVED"] || open["LOCATION_AMBIGUOUS"] || open["OUTSIDE_MICHIGAN"] {
		status = StatusNeedsLocation
	} else if open["GEOMETRY_UNCONFIRMED"] {
		status = StatusNeedsGeometry
	} else if len(open) > 0 {
		status = StatusNeedsDetails
	}
	if err := queries.SetCandidateFieldStatus(ctx, db.SetCandidateFieldStatusParams{ID: record.ID, WorkspaceID: record.WorkspaceID, Status: string(status)}); err != nil {
		return fmt.Errorf("set candidate field status: %w", err)
	}
	return nil
}

type gapDefinition struct{ detail, resolution string }

func gapDefinitions() map[string]gapDefinition {
	return map[string]gapDefinition{
		"LOCATION_UNRESOLVED":    {"The field location has not been safely resolved.", "Use an address or coordinates Mireye can resolve, or upload the field boundary."},
		"LOCATION_AMBIGUOUS":     {"More than one location matches this field.", "Select the correct location or upload the field boundary."},
		"OUTSIDE_MICHIGAN":       {"The resolved location is outside Michigan.", "Correct the locator or upload the Michigan field boundary."},
		"GEOMETRY_UNCONFIRMED":   {"The actual application boundary is not confirmed.", "Confirm the matched parcel or upload the actual field boundary."},
		"RMP_APPROVAL_MISSING":   {"RMP approval is not confirmed.", "Confirm the field is approved in the Residuals Management Program."},
		"USABLE_ACRES_MISSING":   {"Usable application acres are missing.", "Enter the approved usable acreage."},
		"AGRONOMIC_RATE_MISSING": {"The operator's agronomic rate is missing.", "Enter the dry tons per acre allowed by the agronomic plan."},
		"PRIOR_LOADING_MISSING":  {"Prior biosolids loading is missing.", "Enter prior dry tons, including zero when none were applied."},
	}
}

func (s *Service) list(ctx context.Context, queries *db.Queries, workspaceID uuid.UUID, facilityID, fieldID *string) ([]Field, error) {
	params := db.ListCandidateFieldRowsParams{WorkspaceID: workspaceID}
	if facilityID != nil && strings.TrimSpace(*facilityID) != "" {
		parsed, err := uuid.Parse(*facilityID)
		if err != nil {
			return nil, ErrNotFound
		}
		params.FacilityID = uuid.NullUUID{UUID: parsed, Valid: true}
	}
	if fieldID != nil {
		parsed, err := uuid.Parse(*fieldID)
		if err != nil {
			return nil, ErrNotFound
		}
		params.FieldID = uuid.NullUUID{UUID: parsed, Valid: true}
	}
	rows, err := queries.ListCandidateFieldRows(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list candidate fields: %w", err)
	}
	gapRows, err := queries.ListOpenFieldGapsForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list field gaps: %w", err)
	}
	gaps := make(map[uuid.UUID][]FieldGap)
	for _, gap := range gapRows {
		gaps[gap.FieldID] = append(gaps[gap.FieldID], FieldGap{ID: gap.ID.String(), Code: gap.Code, Detail: gap.Detail, Resolution: gap.Resolution, CreatedAt: gap.CreatedAt.Time})
	}
	fields := make([]Field, 0, len(rows))
	for _, row := range rows {
		candidateField, err := fieldFromRow(row, gaps[row.ID])
		if err != nil {
			return nil, err
		}
		fields = append(fields, candidateField)
	}
	return fields, nil
}

func fieldFromRow(row db.ListCandidateFieldRowsRow, gaps []FieldGap) (Field, error) {
	result := Field{
		ID:       row.ID.String(),
		Facility: FieldFacility{ID: row.FacilityID.String(), Name: row.FacilityName, Jurisdiction: "MI"},
		Name:     row.Name, LocatorKind: LocatorKind(row.LocatorKind), LocatorInput: row.LocatorInput,
		Status: Status(row.Status),
		Details: Details{
			MiEnviroSiteID: row.MienviroSiteID, RMPApproved: row.RmpApproved,
			RMPDocumentReference: row.RmpDocumentReference, UsableAcres: optional(row.UsableAcres),
			CropOrUse: row.CropOrUse, AgronomicRateDryTonsPerAcre: optional(row.AgronomicRateDryTonsAcre),
			PriorLoadingDryTons: optional(row.PriorLoadingDryTons), KnownConstraints: row.KnownConstraints,
			AccessConstraints: row.AccessConstraints,
		},
		Gaps: gaps, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if result.Gaps == nil {
		result.Gaps = []FieldGap{}
	}
	if row.LookupID != "" {
		var candidates []Candidate
		if err := json.Unmarshal(row.Candidates, &candidates); err != nil {
			return Field{}, fmt.Errorf("decode location choices for field %s: %w", row.ID, err)
		}
		location := &Location{
			ID: row.LookupID, Disposition: row.LookupDisposition,
			Latitude: optional(row.Latitude), Longitude: optional(row.Longitude),
			ResolvedAddress: dereference(row.ResolvedAddress), State: dereference(row.State),
			County: dereference(row.County), FIPS: dereference(row.Fips), Confidence: optional(row.Confidence),
			MatchMethod: dereference(row.MatchMethod), ParcelUnavailable: row.ParcelUnavailable,
			Candidates: candidates, Reason: dereference(row.Reason), Hint: dereference(row.Hint),
			RequestID: dereference(row.RequestID), SourceURL: row.SourceUrl,
			ResponseHash: row.ResponseHash, FetchedAt: row.FetchedAt.Time,
		}
		if len(row.ParcelGeometry) > 0 || row.ParcelID != nil {
			location.Parcel = &Parcel{
				ID: dereference(row.ParcelID), Geometry: row.ParcelGeometry,
				MatchType: dereference(row.ParcelMatchType), MatchDistanceM: optional(row.ParcelMatchDistanceM),
				Source: dereference(row.ParcelSource),
			}
		}
		result.Location = location
	}
	if row.GeometryVersion != nil {
		var confirmedAt *time.Time
		if row.GeometryConfirmedAt.Valid {
			value := row.GeometryConfirmedAt.Time
			confirmedAt = &value
		}
		result.Geometry = &Geometry{
			Version: int(*row.GeometryVersion), Source: dereference(row.GeometrySource),
			GeoJSON: json.RawMessage(row.GeometryGeojson), AreaAcres: row.GeometryAreaAcres,
			Hash: dereference(row.GeometryHash), Confirmed: row.GeometryConfirmedAt.Valid, ConfirmedAt: confirmedAt,
		}
	}
	return result, nil
}

func (s *Service) loadBase(ctx context.Context, workspaceKey, fieldID string) (db.PfasWorkspace, uuid.UUID, db.PfasCandidateField, error) {
	workspaceRecord, err := s.workspace(ctx, workspaceKey)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, db.PfasCandidateField{}, err
	}
	id, err := uuid.Parse(fieldID)
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, db.PfasCandidateField{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetCandidateFieldBase(ctx, db.GetCandidateFieldBaseParams{ID: id, WorkspaceID: workspaceRecord.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PfasWorkspace{}, uuid.Nil, db.PfasCandidateField{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, uuid.Nil, db.PfasCandidateField{}, fmt.Errorf("load candidate field: %w", err)
	}
	return workspaceRecord, id, record, nil
}

func (s *Service) workspace(ctx context.Context, key string) (db.PfasWorkspace, error) {
	keyHash, err := workspace.Hash(key)
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("%w: workspace key is malformed", ErrInvalid)
	}
	record, err := db.New(s.pool).GetWorkspaceByHash(ctx, keyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PfasWorkspace{}, ErrNotFound
	}
	if err != nil {
		return db.PfasWorkspace{}, fmt.Errorf("load private workspace: %w", err)
	}
	return record, nil
}

func validateCreateInput(input CreateInput) (CreateInput, string, json.RawMessage, string, error) {
	input.Name = strings.Join(strings.Fields(input.Name), " ")
	input.Locator = strings.TrimSpace(input.Locator)
	input.County = strings.Join(strings.Fields(input.County), " ")
	if len(input.Name) < 1 || len(input.Name) > 160 {
		return input, "", nil, "", fmt.Errorf("%w: field name is required and must be at most 160 characters", ErrInvalid)
	}
	switch input.LocatorKind {
	case LocatorAddress, LocatorCoordinate:
		if len(input.Locator) < 1 || len(input.Locator) > 256 {
			return input, "", nil, "", fmt.Errorf("%w: enter an address or coordinates no longer than 256 characters", ErrInvalid)
		}
		return input, input.Locator, nil, "", nil
	case LocatorAPN:
		if input.Locator == "" || input.County == "" {
			return input, "", nil, "", fmt.Errorf("%w: APN and county are both required", ErrInvalid)
		}
		locator := input.Locator + ", " + input.County + " County, Michigan"
		if len(locator) > 256 {
			return input, "", nil, "", fmt.Errorf("%w: APN and county are too long", ErrInvalid)
		}
		return input, locator, nil, "", nil
	case LocatorGeoJSON:
		geometry, geometryHash, err := canonicalGeometry(input.GeoJSON)
		if err != nil {
			return input, "", nil, "", fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return input, "Uploaded field boundary", geometry, geometryHash, nil
	default:
		return input, "", nil, "", fmt.Errorf("%w: choose address, coordinates, APN, or GeoJSON", ErrInvalid)
	}
}

func cleanDetails(input *DetailsInput) {
	input.MiEnviroSiteID = cleanOptional(input.MiEnviroSiteID)
	input.RMPDocumentReference = cleanOptional(input.RMPDocumentReference)
	input.UsableAcres = cleanOptional(input.UsableAcres)
	input.CropOrUse = cleanOptional(input.CropOrUse)
	input.AgronomicRateDryTonsPerAcre = cleanOptional(input.AgronomicRateDryTonsPerAcre)
	input.PriorLoadingDryTons = cleanOptional(input.PriorLoadingDryTons)
	input.KnownConstraints = cleanOptional(input.KnownConstraints)
	input.AccessConstraints = cleanOptional(input.AccessConstraints)
}

func decimal(value *string, allowZero bool) (pgtype.Numeric, error) {
	if value == nil {
		return pgtype.Numeric{}, nil
	}
	trimmed := strings.TrimSpace(*value)
	if !decimalPattern.MatchString(trimmed) {
		return pgtype.Numeric{}, errors.New("invalid decimal")
	}
	rational, ok := new(big.Rat).SetString(trimmed)
	if !ok || rational.Sign() < 0 || (!allowZero && rational.Sign() == 0) {
		return pgtype.Numeric{}, errors.New("decimal is outside the allowed range")
	}
	var result pgtype.Numeric
	if err := result.Scan(trimmed); err != nil {
		return pgtype.Numeric{}, err
	}
	return result, nil
}

func floatNumeric(value *float64) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	var result pgtype.Numeric
	_ = result.Scan(strconv.FormatFloat(*value, 'f', -1, 64))
	return result
}

func mapLookupKind(kind LocatorKind) mireye.LookupKind {
	switch kind {
	case LocatorAddress:
		return mireye.LookupAddress
	case LocatorCoordinate:
		return mireye.LookupCoordinate
	default:
		return mireye.LookupAPN
	}
}

func isMichigan(state *string) bool {
	if state == nil {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(*state))
	return value == "mi" || value == "michigan"
}

func normalize(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), " ")) }

func optional(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	return optional(*value)
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isUniqueViolation(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == "23505"
}
