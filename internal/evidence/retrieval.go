package evidence

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
)

func (s *Service) persistBatch(ctx context.Context, evaluationID uuid.UUID, batch mireye.FetchBatchResult, sampleIndices []int, fields []string) error {
	queries := db.New(s.pool)
	if err := queries.CreateMireyeFetchBatch(ctx, db.CreateMireyeFetchBatchParams{
		ID: uuid.New(), EvaluationID: evaluationID, RequestHash: batch.RequestHash,
		ResponseHash: batch.ResponseHash, RequestID: optionalString(batch.RequestID), SourceUrl: batch.SourceURL,
		HttpStatus: int32(batch.HTTPStatus), Request: batch.Request, Response: batch.Raw,
		FetchedAt: pgTimestamp(&batch.FetchedAt),
	}); err != nil {
		return fmt.Errorf("store Mireye batch evidence: %w", err)
	}
	for localIndex, item := range batch.Results {
		if localIndex >= len(sampleIndices) {
			return fmt.Errorf("mireye batch result exceeded the stored sample mapping")
		}
		sampleIndex := int32(sampleIndices[localIndex])
		for _, fieldName := range fields {
			fact := normalizedField(item, fieldName)
			var ttl *int32
			if fact.TTLSeconds != nil {
				value := int32(*fact.TTLSeconds)
				ttl = &value
			}
			if err := queries.UpsertPhysicalSampleFact(ctx, db.UpsertPhysicalSampleFactParams{
				EvaluationID: evaluationID, SampleIndex: sampleIndex, FieldName: fieldName,
				Status: fact.Status, Value: fact.Value, Unit: fact.Unit, Source: fact.Source,
				SourceUrl: fact.SourceURL, Confidence: fact.Confidence, FetchedAt: pgTimestamp(fact.FetchedAt),
				DatasetVintage: fact.DatasetVintage, TtlSeconds: ttl, Notes: fact.Notes,
				Error: fact.Error, Retryable: fact.Retryable,
			}); err != nil {
				return fmt.Errorf("store Mireye sample fact %s: %w", fieldName, err)
			}
		}
	}
	return nil
}

func normalizedField(item mireye.FetchLocationResult, fieldName string) mireye.FetchField {
	if !item.OK {
		message := "Mireye could not evaluate this sample location"
		retryable := false
		if item.Error != nil {
			message = strings.TrimSpace(item.Error.Code + ": " + item.Error.Message)
			retryable = item.Error.Retryable
		}
		return mireye.FetchField{Status: "failed", Error: &message, Retryable: retryable}
	}
	fact, ok := item.Fields[fieldName]
	if !ok {
		message := "Mireye omitted this requested field"
		return mireye.FetchField{Status: "failed", Error: &message}
	}
	if fact.Status != "ok" && fact.Status != "absent" && fact.Status != "failed" {
		message := "Mireye returned an unknown field status"
		fact.Status, fact.Error, fact.Retryable = "failed", &message, false
	}
	if fact.Status == "ok" && (len(fact.Value) == 0 || bytes.Equal(bytes.TrimSpace(fact.Value), []byte("null"))) {
		message := "Mireye marked a null value as available"
		fact.Status, fact.Error, fact.Retryable = "failed", &message, false
	}
	return fact
}

func (s *Service) retryFailedFields(ctx context.Context, evaluationID uuid.UUID, samples []db.GeneratePhysicalSamplePointsRow, fetchPerField int) error {
	rows, err := db.New(s.pool).ListPhysicalSampleFacts(ctx, evaluationID)
	if err != nil {
		return fmt.Errorf("load retryable physical facts: %w", err)
	}
	retryable := make(map[string][]int)
	for _, row := range rows {
		if row.Status == "failed" && row.Retryable {
			retryable[row.FieldName] = append(retryable[row.FieldName], int(row.SampleIndex))
		}
	}
	remainingCredits := maxProjectedCredits - (len(samples) * len(physicalFieldSpecs) * fetchPerField)
	for _, spec := range physicalFieldSpecs {
		indices := retryable[spec.Name]
		retryCost := len(indices) * fetchPerField
		if len(indices) == 0 || retryCost > remainingCredits {
			continue
		}
		request := mireye.FetchBatchRequest{Fields: []string{spec.Name}, Locations: make([]mireye.Coordinate, len(indices))}
		for index, sampleIndex := range indices {
			if sampleIndex < 0 || sampleIndex >= len(samples) {
				return fmt.Errorf("retryable sample index %d is invalid", sampleIndex)
			}
			request.Locations[index] = mireye.Coordinate{Latitude: samples[sampleIndex].Latitude, Longitude: samples[sampleIndex].Longitude}
		}
		batch, err := s.mireye.FetchBatch(ctx, request)
		if err != nil {
			s.logger.Warn("Mireye field retry unavailable", "evaluation_id", evaluationID.String(), "field", spec.Name, "error_type", fmt.Sprintf("%T", err))
			continue
		}
		if err := s.persistBatch(ctx, evaluationID, batch, indices, request.Fields); err != nil {
			return err
		}
		remainingCredits -= retryCost
	}
	return nil
}
