package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
)

const (
	wellogicPublicURL = "https://www.arcgis.com/home/item.html?id=58c98df11b6a411c97b8aeb839a695ad"
	mienviroPublicURL = "https://www.michigan.gov/egle/maps-data/mienviroportal"
)

func (s *Service) collectSupplemental(ctx context.Context, evaluationID uuid.UUID, work db.GetPhysicalEvaluationForWorkRow) error {
	queries := db.New(s.pool)
	envelope, err := queries.GetFieldGeometryEnvelope(ctx, db.GetFieldGeometryEnvelopeParams{ID: work.GeometryID, FieldID: work.FieldID})
	if err != nil {
		return fmt.Errorf("load field envelope for adjacent evidence: %w", err)
	}
	wellogic, wellogicErr := s.supplemental.FetchWellogic(ctx, Envelope{
		MinLongitude: envelope.MinLng, MinLatitude: envelope.MinLat,
		MaxLongitude: envelope.MaxLng, MaxLatitude: envelope.MaxLat,
	})
	if wellogicErr != nil {
		if err := queries.UpsertSupplementalEvidence(ctx, db.UpsertSupplementalEvidenceParams{
			EvaluationID: evaluationID, Provider: "WELLOGIC", Kind: "MAPPED_WELLS", Status: "UNAVAILABLE",
			Title: "Michigan mapped wells", Summary: "The current Wellogic spatial query could not be completed.",
			SourceUrl: wellogicPublicURL, FetchedAt: pgTimestamp(timePointerValue(time.Now().UTC())),
			Caveat: optionalString("No absence was inferred. Wellogic does not contain every well, especially older records."),
		}); err != nil {
			return fmt.Errorf("record unavailable Wellogic evidence: %w", err)
		}
		if err := queries.UpsertPhysicalDataGap(ctx, db.UpsertPhysicalDataGapParams{
			EvaluationID: evaluationID, Code: "WELLOGIC_UNAVAILABLE", Source: "Michigan Wellogic",
			Detail: "Mapped Michigan well records could not be checked for this evaluation.", Critical: false,
		}); err != nil {
			return fmt.Errorf("record Wellogic data gap: %w", err)
		}
	} else if err := s.storeWellogic(ctx, evaluationID, work, wellogic); err != nil {
		return err
	}

	samples, err := queries.ListPhysicalSamplePoints(ctx, evaluationID)
	if err != nil || len(samples) == 0 {
		return fmt.Errorf("load anchor for EPA ECHO evidence: %w", err)
	}
	echo, echoErr := s.supplemental.FetchECHOPotentialSectors(ctx, samples[0].Latitude, samples[0].Longitude)
	if echoErr != nil {
		if err := queries.UpsertSupplementalEvidence(ctx, db.UpsertSupplementalEvidenceParams{
			EvaluationID: evaluationID, Provider: "EPA_ECHO", Kind: "POTENTIAL_PFAS_SECTORS", Status: "UNAVAILABLE",
			Title: "Nearby regulated industries", Summary: "The current EPA ECHO query could not be completed.",
			SourceUrl: echoSectorURL, SourceVintage: optionalString("Sector list updated March 2023"),
			FetchedAt: pgTimestamp(timePointerValue(time.Now().UTC())),
			Caveat:    optionalString("No absence was inferred. Industry-sector screening is not evidence that a facility used or released PFAS."),
		}); err != nil {
			return fmt.Errorf("record unavailable EPA ECHO evidence: %w", err)
		}
		if err := queries.UpsertPhysicalDataGap(ctx, db.UpsertPhysicalDataGapParams{
			EvaluationID: evaluationID, Code: "EPA_ECHO_UNAVAILABLE", Source: "EPA ECHO",
			Detail: "Nearby regulated industries could not be checked for this evaluation.", Critical: false,
		}); err != nil {
			return fmt.Errorf("record EPA ECHO data gap: %w", err)
		}
	} else if err := s.storeECHO(ctx, evaluationID, echo); err != nil {
		return err
	}
	return s.storeOperatorRecord(ctx, evaluationID, work.WorkspaceID, work.FieldID)
}

func (s *Service) storeWellogic(ctx context.Context, evaluationID uuid.UUID, work db.GetPhysicalEvaluationForWorkRow, result WellogicResult) error {
	queries := db.New(s.pool)
	for index := range result.Wells {
		distance, err := queries.DistanceFromFieldToPointMeters(ctx, db.DistanceFromFieldToPointMetersParams{
			GeometryID: work.GeometryID, FieldID: work.FieldID,
			Longitude: result.Wells[index].Longitude, Latitude: result.Wells[index].Latitude,
		})
		if err != nil {
			return fmt.Errorf("measure field-to-well distance: %w", err)
		}
		result.Wells[index].DistanceM = distance
	}
	sort.Slice(result.Wells, func(left, right int) bool { return result.Wells[left].DistanceM < result.Wells[right].DistanceM })
	total := len(result.Wells)
	if len(result.Wells) > 10 {
		result.Wells = result.Wells[:10]
	}
	summary := "No mapped Wellogic wells were returned near the field."
	if total > 0 {
		summary = strconv.Itoa(total) + " mapped well"
		if total != 1 {
			summary += "s"
		}
		summary += " were returned near the field; the nearest is " + formatMeters(result.Wells[0].DistanceM) + " from its boundary."
	}
	value, err := json.Marshal(map[string]any{"count": total, "nearest": result.Wells})
	if err != nil {
		return fmt.Errorf("encode Wellogic evidence: %w", err)
	}
	return queries.UpsertSupplementalEvidence(ctx, db.UpsertSupplementalEvidenceParams{
		EvaluationID: evaluationID, Provider: "WELLOGIC", Kind: "MAPPED_WELLS", Status: "AVAILABLE",
		Title: "Michigan mapped wells", Summary: summary, Value: value, SourceUrl: wellogicPublicURL,
		SourceVintage: optionalString(result.SourceVintage), FetchedAt: pgTimestamp(&result.FetchedAt),
		Caveat: optionalString("A mapped well is evidence of a known record. No result is not proof that no well exists; older records are incomplete and location quality varies."),
	})
}

func (s *Service) storeECHO(ctx context.Context, evaluationID uuid.UUID, result ECHOResult) error {
	total := len(result.Facilities)
	displayed := result.Facilities
	if len(displayed) > 10 {
		displayed = displayed[:10]
	}
	summary := "No regulated facilities with a listed potential PFAS-handling industry code were returned within 5 miles."
	if total > 0 {
		summary = strconv.Itoa(total) + " regulated facilit"
		if total == 1 {
			summary += "y has"
		} else {
			summary += "ies have"
		}
		summary += " a listed potential PFAS-handling industry code within 5 miles."
	}
	value, err := json.Marshal(map[string]any{"count": total, "queryRows": result.QueryRows, "facilities": displayed, "truncated": result.Truncated})
	if err != nil {
		return fmt.Errorf("encode EPA ECHO evidence: %w", err)
	}
	return db.New(s.pool).UpsertSupplementalEvidence(ctx, db.UpsertSupplementalEvidenceParams{
		EvaluationID: evaluationID, Provider: "EPA_ECHO", Kind: "POTENTIAL_PFAS_SECTORS", Status: "AVAILABLE",
		Title: "Nearby regulated industries", Summary: summary, Value: value, SourceUrl: echoSectorURL,
		SourceVintage: optionalString("Sector list updated March 2023"), FetchedAt: pgTimestamp(&result.FetchedAt),
		Caveat: optionalString("This is an investigation lead, not proof that a listed facility manufactured, used, or released PFAS, and it does not establish causation."),
	})
}

func (s *Service) storeOperatorRecord(ctx context.Context, evaluationID, workspaceID, fieldID uuid.UUID) error {
	record, err := db.New(s.pool).GetCandidateFieldBase(ctx, db.GetCandidateFieldBaseParams{ID: fieldID, WorkspaceID: workspaceID})
	if err != nil {
		return fmt.Errorf("load operator approval evidence: %w", err)
	}
	return s.upsertOperatorRecord(ctx, evaluationID, record.MienviroSiteID, record.RmpApproved, record.RmpDocumentReference, record.UpdatedAt.Time)
}

func (s *Service) upsertOperatorRecord(ctx context.Context, evaluationID uuid.UUID, siteID *string, approved *bool, reference *string, updatedAt time.Time) error {
	status := "NOT_PROVIDED"
	summary := "No MiEnviro or RMP reference was supplied for this field."
	if approved != nil && *approved {
		status = "AVAILABLE"
		summary = "The operator confirmed this field is approved in the Residuals Management Program."
	}
	value, err := json.Marshal(map[string]any{"miEnviroSiteId": siteID, "rmpApproved": approved, "documentReference": reference})
	if err != nil {
		return fmt.Errorf("encode operator approval evidence: %w", err)
	}
	return db.New(s.pool).UpsertSupplementalEvidence(ctx, db.UpsertSupplementalEvidenceParams{
		EvaluationID: evaluationID, Provider: "OPERATOR_RECORD", Kind: "MIENVIRO_RMP", Status: status,
		Title: "Field approval record", Summary: summary, Value: value, SourceUrl: mienviroPublicURL,
		SourceVintage: optionalString(updatedAt.UTC().Format(time.RFC3339)), FetchedAt: pgTimestamp(&updatedAt),
		Caveat: optionalString("Operator confirmation is preserved separately from Mireye physical facts and should be checked against the cited approval record."),
	})
}

func formatMeters(value float64) string {
	if value >= 1000 {
		return strconv.FormatFloat(value/1000, 'f', 1, 64) + " km"
	}
	return strconv.FormatFloat(value, 'f', 0, 64) + " m"
}

func timePointerValue(value time.Time) *time.Time { return &value }
