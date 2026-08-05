package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
)

func (s *Service) aggregate(ctx context.Context, evaluationID uuid.UUID, sampleCount int) (bool, error) {
	queries := db.New(s.pool)
	rows, err := queries.ListPhysicalSampleFacts(ctx, evaluationID)
	if err != nil {
		return false, fmt.Errorf("load physical sample facts for aggregation: %w", err)
	}
	byField := make(map[string][]sampleObservation)
	for _, row := range rows {
		byField[row.FieldName] = append(byField[row.FieldName], sampleObservation{
			Index: int(row.SampleIndex), Status: row.Status, Value: row.Value,
			Unit: stringValue(row.Unit), Source: stringValue(row.Source), SourceURL: stringValue(row.SourceUrl),
			FetchedAt: timePointer(row.FetchedAt),
		})
	}
	hasCriticalGaps := false
	for _, spec := range physicalFieldSpecs {
		result, aggregateErr := aggregateField(spec, byField[spec.Name], sampleCount)
		if aggregateErr != nil {
			result = aggregateResult{State: "UNAVAILABLE", FailedCount: sampleCount, SampleIndices: []int{}}
			if err := queries.UpsertPhysicalDataGap(ctx, db.UpsertPhysicalDataGapParams{
				EvaluationID: evaluationID, Code: "MIREYE_INVALID_VALUE_" + strings.ToUpper(spec.Name),
				Source: "Mireye", Detail: "Mireye returned a value that could not be safely aggregated for " + spec.Label + ".",
				Critical: spec.Critical,
			}); err != nil {
				return false, fmt.Errorf("record invalid Mireye value gap: %w", err)
			}
		}
		indices, err := json.Marshal(result.SampleIndices)
		if err != nil {
			return false, fmt.Errorf("encode aggregate sample indices: %w", err)
		}
		if err := queries.UpsertPhysicalFieldFact(ctx, db.UpsertPhysicalFieldFactParams{
			EvaluationID: evaluationID, FieldName: spec.Name, Category: spec.Category,
			Label: spec.Label, State: result.State, AggregateMethod: string(spec.Method),
			Value: result.Value, Unit: optionalString(result.Unit), Source: optionalString(result.Source),
			SourceUrl: optionalString(result.SourceURL), FetchedAt: pgTimestamp(result.FetchedAt),
			OkCount: int32(result.OKCount), AbsentCount: int32(result.AbsentCount),
			FailedCount: int32(result.FailedCount), SampleIndices: indices, Critical: spec.Critical,
		}); err != nil {
			return false, fmt.Errorf("store aggregate physical fact %s: %w", spec.Name, err)
		}
		if result.State != "COMPLETE" {
			if spec.Critical {
				hasCriticalGaps = true
			}
			code := "MIREYE_" + result.State + "_" + strings.ToUpper(spec.Name)
			detail := spec.Label + " is incomplete across the confirmed field samples."
			if result.State == "UNAVAILABLE" {
				detail = spec.Label + " is unavailable across the confirmed field samples."
			}
			if err := queries.UpsertPhysicalDataGap(ctx, db.UpsertPhysicalDataGapParams{
				EvaluationID: evaluationID, Code: code, Source: "Mireye", Detail: detail, Critical: spec.Critical,
			}); err != nil {
				return false, fmt.Errorf("record aggregate data gap: %w", err)
			}
		}
	}
	return hasCriticalGaps, nil
}

func (s *Service) build(ctx context.Context, record db.PfasPhysicalEvaluation) (Evaluation, error) {
	queries := db.New(s.pool)
	work, err := queries.GetPhysicalEvaluationForWork(ctx, record.ID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("load physical evaluation field context: %w", err)
	}
	pointRows, err := queries.ListPhysicalSamplePoints(ctx, record.ID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("load physical sample points: %w", err)
	}
	factRows, err := queries.ListPhysicalFieldFacts(ctx, record.ID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("load physical field facts: %w", err)
	}
	sampleRows, err := queries.ListPhysicalSampleFacts(ctx, record.ID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("load physical sample evidence: %w", err)
	}
	supplementalRows, err := queries.ListSupplementalEvidence(ctx, record.ID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("load supplemental field evidence: %w", err)
	}
	gapRows, err := queries.ListPhysicalDataGaps(ctx, record.ID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("load physical data gaps: %w", err)
	}

	result := Evaluation{
		ID: record.ID.String(), FieldID: record.FieldID.String(), GeometryVersion: int(work.GeometryVersion),
		Status: Status(record.Status), FieldSetVersion: record.FieldSetVersion,
		AggregationVersion: record.AggregationVersion, CatalogVersion: stringValue(record.CatalogVersion),
		SampleCount: int(record.SampleCount), ProjectedCredits: int(record.ProjectedCredits),
		FailureCode: stringValue(record.FailureCode), FailureDetail: stringValue(record.FailureDetail),
		Samples: []SamplePoint{}, Facts: []FieldFact{}, Supplemental: []SupplementalEvidence{}, Gaps: []PhysicalDataGap{},
		StartedAt: timePointer(record.StartedAt), CompletedAt: timePointer(record.CompletedAt),
		CreatedAt: record.CreatedAt.Time, UpdatedAt: record.UpdatedAt.Time,
	}
	for _, point := range pointRows {
		result.Samples = append(result.Samples, SamplePoint{Index: int(point.SampleIndex), Label: point.Label, Latitude: point.Latitude, Longitude: point.Longitude})
	}
	samplesByField := make(map[string][]SampleFact)
	for _, row := range sampleRows {
		samplesByField[row.FieldName] = append(samplesByField[row.FieldName], SampleFact{
			Index: int(row.SampleIndex), Label: row.SampleLabel, Latitude: row.Latitude, Longitude: row.Longitude,
			Status: row.Status, Value: rawValue(row.Value), Unit: stringValue(row.Unit), Source: stringValue(row.Source),
			SourceURL: stringValue(row.SourceUrl), Confidence: stringValue(row.Confidence), FetchedAt: timePointer(row.FetchedAt),
			DatasetVintage: stringValue(row.DatasetVintage), Notes: stringValue(row.Notes), Error: stringValue(row.Error),
		})
	}
	factsByName := make(map[string]db.PfasPhysicalFieldFact, len(factRows))
	for _, row := range factRows {
		factsByName[row.FieldName] = row
	}
	for _, spec := range physicalFieldSpecs {
		row, ok := factsByName[spec.Name]
		if !ok {
			continue
		}
		result.Facts = append(result.Facts, FieldFact{
			Name: row.FieldName, Label: row.Label, Category: row.Category, State: row.State,
			AggregateMethod: row.AggregateMethod, Value: rawValue(row.Value), Unit: stringValue(row.Unit),
			Source: stringValue(row.Source), SourceURL: stringValue(row.SourceUrl), FetchedAt: timePointer(row.FetchedAt),
			OKCount: int(row.OkCount), AbsentCount: int(row.AbsentCount), FailedCount: int(row.FailedCount),
			Critical: row.Critical, Samples: samplesByField[row.FieldName],
		})
	}
	for _, row := range supplementalRows {
		result.Supplemental = append(result.Supplemental, SupplementalEvidence{
			Provider: row.Provider, Kind: row.Kind, Status: row.Status, Title: row.Title,
			Summary: row.Summary, Value: rawValue(row.Value), SourceURL: row.SourceUrl,
			SourceVintage: stringValue(row.SourceVintage), FetchedAt: row.FetchedAt.Time, Caveat: stringValue(row.Caveat),
		})
	}
	for _, row := range gapRows {
		result.Gaps = append(result.Gaps, PhysicalDataGap{Code: row.Code, Source: row.Source, Detail: row.Detail, Critical: row.Critical})
	}
	return result, nil
}

func rawValue(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return json.RawMessage(value)
}
