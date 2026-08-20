package placement

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"
)

type testEvidenceStore map[string]EvidenceArtifact

func (s testEvidenceStore) LoadSlopeEvidence(_ context.Context, id string) (EvidenceArtifact, error) {
	record, ok := s[id]
	if !ok {
		return EvidenceArtifact{}, errors.New("record not found")
	}
	return record, nil
}

func TestVerifySlopeResolutionAcceptsImmutableGeometryEvidence(t *testing.T) {
	field, store, asOf := validResolutionFixture(t, eligibleField("slope", "Slope field", "50", "1", "0", 20, "grain"))
	verified, err := VerifySlopeResolution(context.Background(), field, store, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if verified.DerivedUsableAcres != "31.6" || verified.HighSlopeSamplesExcluded != 1 || verified.BoundaryVersion != 4 || verified.ParentBoundaryVersion != 3 || verified.RevisedScreening.ReturnedSampleCount != 5 || verified.RevisedScreening.Status != "SAMPLED_TERRAIN_SCREEN_PASSED" || verified.SlopeConversion.DerivedGradePercent != "16.6" || verified.ProgramPolicyVersion != DemoProgramPolicyVersion || !verified.DemonstrationRoles {
		t.Fatalf("unexpected verification: %#v", verified)
	}
}

func TestVerifySlopeResolutionRejectsInvalidReferencesAndRecords(t *testing.T) {
	tests := []struct {
		name string
		code string
		edit func(*testing.T, *FieldInput, testEvidenceStore)
	}{
		{name: "unknown ID", code: "UNKNOWN_EVIDENCE_ID", edit: func(_ *testing.T, field *FieldInput, _ testEvidenceStore) {
			field.SlopeResolution.EvidenceRecordID = "b4c0b3a1-6f74-4cad-9df1-7d7bc68d1999"
		}},
		{name: "changed artifact hash", code: "ARTIFACT_HASH_MISMATCH", edit: func(_ *testing.T, field *FieldInput, _ testEvidenceStore) {
			field.SlopeResolution.ArtifactHash = "f" + field.SlopeResolution.ArtifactHash[1:]
		}},
		{name: "incorrect evidence type", code: "UNSUPPORTED_EVIDENCE_TYPE", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.EvidenceType = "DOCUMENTED_EGLE_APPROVAL" })
			field.SlopeResolution.EvidenceType = "DOCUMENTED_EGLE_APPROVAL"
		}},
		{name: "wrong field", code: "FIELD_MISMATCH", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.FieldID = "another-field" })
		}},
		{name: "unconfigured issuer role", code: "UNAUTHORIZED_ISSUER", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.IssuingRole = "UNCONFIGURED_ROLE" })
		}},
		{name: "stale boundary version", code: "BOUNDARY_VERSION_MISMATCH", edit: func(_ *testing.T, field *FieldInput, _ testEvidenceStore) {
			field.BoundaryVersion = 5
		}},
		{name: "unconfigured reviewer role", code: "UNAUTHORIZED_REVIEWER", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.ReviewerRole = "OPERATOR" })
		}},
		{name: "unconfirmed evidence", code: "EVIDENCE_NOT_CONFIRMED", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.ConfirmationStatus = "DRAFT" })
		}},
		{name: "expired evidence", code: "EVIDENCE_EXPIRED", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) {
				artifact.ExpiresAt = time.Date(2026, time.August, 20, 7, 59, 0, 0, time.UTC)
			})
		}},
		{name: "superseded evidence", code: "EVIDENCE_SUPERSEDED", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.SupersededBy = "replacement-record" })
		}},
		{name: "changed source reference hash", code: "SOURCE_REFERENCE_MISMATCH", edit: func(_ *testing.T, field *FieldInput, _ testEvidenceStore) {
			field.SlopeResolution.SourceArtifactHash = "0" + field.SlopeResolution.SourceArtifactHash[1:]
		}},
		{name: "changed source artifact", code: "SOURCE_HASH_MISMATCH", edit: func(_ *testing.T, field *FieldInput, store testEvidenceStore) {
			record := store[field.SlopeResolution.SourceEvidenceRecordID]
			record.Artifact = append([]byte(nil), record.Artifact...)
			record.Artifact[0] ^= 0xff
			store[record.ID] = record
		}},
		{name: "valid polygon somewhere else", code: "REVISED_BOUNDARY_OUTSIDE_PARENT", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) {
				artifact.Boundary.Coordinates = [][][]float64{{{-85.004, 43.001}, {-85, 43.001}, {-85, 43.004536}, {-85.004, 43.004536}, {-85.004, 43.001}}}
			})
		}},
		{name: "polygon extends beyond parent", code: "REVISED_BOUNDARY_OUTSIDE_PARENT", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) {
				artifact.Boundary.Coordinates = [][][]float64{{{-84.560, 42.7315}, {-84.5545, 42.7315}, {-84.5545, 42.735036}, {-84.560, 42.735036}, {-84.560, 42.7315}}}
			})
		}},
		{name: "changed parent boundary hash", code: "PARENT_BOUNDARY_HASH_MISMATCH", edit: func(_ *testing.T, field *FieldInput, store testEvidenceStore) {
			record := store[field.SlopeResolution.ParentBoundaryEvidenceRecordID]
			record.Artifact = append([]byte(nil), record.Artifact...)
			record.Artifact[0] ^= 0xff
			store[record.ID] = record
		}},
		{name: "stale parent version", code: "PARENT_BOUNDARY_REFERENCE_MISMATCH", edit: func(_ *testing.T, field *FieldInput, _ testEvidenceStore) {
			field.SlopeResolution.ParentBoundaryVersion = 2
		}},
		{name: "empty revised polygon", code: "INVALID_BOUNDARY_GEOMETRY", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.Boundary.Coordinates = nil })
		}},
		{name: "unknown reviewer authorization", code: "UNKNOWN_REVIEW_AUTHORIZATION", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			record := store[field.SlopeResolution.EvidenceRecordID]
			var artifact boundaryResolutionArtifact
			if err := json.Unmarshal(record.Artifact, &artifact); err != nil {
				t.Fatal(err)
			}
			delete(store, artifact.ReviewerAuthorizationRecordID)
		}},
		{name: "changed reviewer authorization", code: "REVIEW_AUTHORIZATION_HASH_MISMATCH", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			authorizationID := authorizationID(t, field, store)
			record := store[authorizationID]
			record.Artifact = append([]byte(nil), record.Artifact...)
			record.Artifact[0] ^= 0xff
			store[authorizationID] = record
		}},
		{name: "review authorization for another artifact", code: "REVIEW_AUTHORIZATION_EVIDENCE_MISMATCH", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateAuthorizationArtifact(t, field, store, func(artifact *reviewerAuthorizationArtifact) { artifact.EvidenceArtifactHash = "another-hash" })
		}},
		{name: "review authorization for stale boundary", code: "REVIEW_AUTHORIZATION_SCOPE_MISMATCH", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateAuthorizationArtifact(t, field, store, func(artifact *reviewerAuthorizationArtifact) { artifact.BoundaryVersion = 3 })
		}},
		{name: "unauthorized approval", code: "UNAUTHORIZED_APPROVAL", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateAuthorizationArtifact(t, field, store, func(artifact *reviewerAuthorizationArtifact) { artifact.ApprovalStatus = "PENDING" })
		}},
		{name: "review authorization before evidence", code: "INVALID_REVIEW_AUTHORIZATION_TIME", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateAuthorizationArtifact(t, field, store, func(artifact *reviewerAuthorizationArtifact) {
				artifact.RecordedAt = time.Date(2026, time.August, 20, 7, 57, 0, 0, time.UTC)
			})
		}},
		{name: "superseded review authorization", code: "REVIEW_AUTHORIZATION_SUPERSEDED", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateAuthorizationArtifact(t, field, store, func(artifact *reviewerAuthorizationArtifact) { artifact.SupersededBy = "replacement-authorization" })
		}},
		{name: "geometry still contains high slope", code: "HIGH_SLOPE_STILL_INSIDE_BOUNDARY", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) {
				artifact.Boundary.Coordinates = [][][]float64{{{-84.559, 42.731}, {-84.551, 42.731}, {-84.551, 42.739}, {-84.559, 42.739}, {-84.559, 42.731}}}
			})
		}},
		{name: "revised polygon has no supporting samples", code: "INADEQUATE_REVISED_SCREENING", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateScreeningArtifact(t, field, store, true, func(artifact *revisedScreeningArtifact) { artifact.Response = json.RawMessage(`{"results":[]}`) })
		}},
		{name: "one Mireye result is missing", code: "INADEQUATE_REVISED_SCREENING", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateScreeningArtifact(t, field, store, true, func(artifact *revisedScreeningArtifact) {
				var response map[string]any
				if err := json.Unmarshal(artifact.Response, &response); err != nil {
					t.Fatal(err)
				}
				results := response["results"].([]any)
				response["results"] = results[:len(results)-1]
				artifact.ReturnedSampleCount = len(results) - 1
				artifact.Response, _ = json.Marshal(response)
			})
		}},
		{name: "returned sample is outside the polygon", code: "INADEQUATE_REVISED_SCREENING", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateScreeningArtifact(t, field, store, true, func(artifact *revisedScreeningArtifact) {
				var response map[string]any
				if err := json.Unmarshal(artifact.Response, &response); err != nil {
					t.Fatal(err)
				}
				response["results"].([]any)[0].(map[string]any)["lng"] = -85.0
				artifact.Response, _ = json.Marshal(response)
			})
		}},
		{name: "revised polygon contains excessive slope sample", code: "REVISED_BOUNDARY_SLOPE_EXCEEDED", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateScreeningArtifact(t, field, store, true, func(artifact *revisedScreeningArtifact) {
				var response map[string]any
				if err := json.Unmarshal(artifact.Response, &response); err != nil {
					t.Fatal(err)
				}
				response["results"].([]any)[0].(map[string]any)["fields"].(map[string]any)["slope_degrees"].(map[string]any)["value"] = 9.0
				artifact.Response, _ = json.Marshal(response)
			})
		}},
		{name: "changed revised Mireye response bytes", code: "REVISED_SCREENING_HASH_MISMATCH", edit: func(t *testing.T, field *FieldInput, store testEvidenceStore) {
			updateScreeningArtifact(t, field, store, false, func(artifact *revisedScreeningArtifact) {
				var response map[string]any
				if err := json.Unmarshal(artifact.Response, &response); err != nil {
					t.Fatal(err)
				}
				response["results"].([]any)[0].(map[string]any)["fields"].(map[string]any)["slope_degrees"].(map[string]any)["value"] = 1.6
				artifact.Response, _ = json.Marshal(response)
			})
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, store, asOf := validResolutionFixture(t, eligibleField("slope", "Slope field", "50", "1", "0", 20, "grain"))
			test.edit(t, &field, store)
			_, err := VerifySlopeResolution(context.Background(), field, store, asOf)
			var resolutionErr *ResolutionError
			if !errors.As(err, &resolutionErr) || resolutionErr.Code != test.code {
				t.Fatalf("error = %v, want code %s", err, test.code)
			}
		})
	}
}

func TestSurfaceSlopeThresholdUsesExactUnitConversion(t *testing.T) {
	threshold := SurfaceSlopeThresholdDegrees()
	if SlopeExceedsSurfaceLimit(threshold) || SlopeExceedsSurfaceLimit(math.Nextafter(threshold, 0)) {
		t.Fatal("a slope at or below six-percent grade was treated as excessive")
	}
	if !SlopeExceedsSurfaceLimit(math.Nextafter(threshold, math.Inf(1))) {
		t.Fatal("a slope above six-percent grade was not treated as excessive")
	}
	conversion := SlopeConversionForDegrees(9.42482852935791)
	if conversion.OriginalDegrees != "9.425" || conversion.DerivedGradePercent != "16.6" || conversion.ThresholdDegrees != "3.43" || conversion.ThresholdGradePercent != "6" {
		t.Fatalf("unexpected visible conversion: %#v", conversion)
	}
}

func TestVerifySlopeResolutionAcceptsRolesFromVersionedProgramPolicy(t *testing.T) {
	field, store, asOf := validResolutionFixture(t, eligibleField("slope", "Slope field", "50", "1", "0", 20, "grain"))
	const issuer = "PROGRAM_BOUNDARY_COORDINATOR"
	const reviewer = "PROGRAM_TECHNICAL_REVIEWER"
	updateBoundaryArtifact(t, &field, store, func(artifact *boundaryResolutionArtifact) {
		artifact.IssuingRole = issuer
		artifact.ReviewerRole = reviewer
	})
	updateAuthorizationArtifact(t, &field, store, func(artifact *reviewerAuthorizationArtifact) {
		artifact.ReviewerRole = reviewer
	})
	policy := CurrentResolutionPolicy()
	policy.DemonstrationRoles = false
	policy.AuthorizedIssuerRoles = []string{issuer}
	policy.AuthorizedReviewerRoles = []string{reviewer}
	verified, err := VerifySlopeResolutionWithPolicy(context.Background(), field, store, asOf, policy)
	if err != nil {
		t.Fatal(err)
	}
	if verified.IssuingRole != issuer || verified.ReviewerRole != reviewer || verified.DemonstrationRoles {
		t.Fatalf("configured roles were not preserved: %#v", verified)
	}
}

func validResolutionFixture(t *testing.T, field FieldInput) (FieldInput, testEvidenceStore, time.Time) {
	t.Helper()
	const evidenceID = "a4c0b3a1-6f74-4cad-9df1-7d7bc68d1001"
	const sourceID = "088804c2-fcb6-4c00-8000-000000000001"
	const parentID = "a4c0b3a1-6f74-4cad-9df1-7d7bc68d1000"
	const screeningID = "a4c0b3a1-6f74-4cad-9df1-7d7bc68d1003"
	source := []byte(`{"results":[{"index":4,"ok":true,"lat":42.738,"lng":-84.552,"fields":{"slope_degrees":{"value":9.42482852935791,"status":"ok"}}}]}`)
	sourceHash := artifactHash(source)
	parent := parentBoundaryArtifact{
		SchemaVersion: "confirmed-field-boundary-v1", ID: parentID, FieldID: field.ID, BoundaryVersion: 3,
		CRS: "EPSG:4326", ConfirmationStatus: "CONFIRMED", RecordedAt: time.Date(2026, time.August, 20, 7, 45, 0, 0, time.UTC),
		Boundary: polygonGeometry{Type: "Polygon", Coordinates: [][][]float64{{{-84.559, 42.731}, {-84.551, 42.731}, {-84.551, 42.739}, {-84.559, 42.739}, {-84.559, 42.731}}}},
	}
	parentBytes, err := json.Marshal(parent)
	if err != nil {
		t.Fatal(err)
	}
	parentHash := artifactHash(parentBytes)
	locations := []map[string]float64{{"lat": 42.733268, "lng": -84.5565}, {"lat": 42.732384, "lng": -84.5575}, {"lat": 42.732384, "lng": -84.5555}, {"lat": 42.734152, "lng": -84.5575}, {"lat": 42.734152, "lng": -84.5555}}
	requestBytes, _ := json.Marshal(map[string]any{"locations": locations, "fields": []string{"slope_degrees"}})
	results := make([]map[string]any, 0, len(locations))
	for index, location := range locations {
		results = append(results, map[string]any{"index": index, "ok": true, "lat": location["lat"], "lng": location["lng"], "fields": map[string]any{"slope_degrees": map[string]any{"value": 1.5, "unit": "degrees", "status": "ok"}}})
	}
	responseBytes, _ := json.Marshal(map[string]any{"results": results})
	screening := revisedScreeningArtifact{
		SchemaVersion: "mireye-revised-boundary-screening-v1", ID: screeningID, FieldID: field.ID, BoundaryVersion: 4,
		Endpoint: "https://api.mireye.com/v1/fetch/batch", RequestID: "req_test_revised", HTTPStatus: 200,
		RetrievedAt: time.Date(2026, time.August, 20, 7, 57, 0, 0, time.UTC), RequestHash: artifactHash(requestBytes), ResponseHash: artifactHash(responseBytes),
		AlgorithmVersion: PolygonSamplingAlgorithmV1, MinimumSpacingMeters: 75, BoundaryNearDistanceM: 120,
		MaxRequestSamples: 8, RequestedSampleCount: 5, ReturnedSampleCount: 5, Request: requestBytes, Response: responseBytes,
	}
	screeningBytes, err := json.Marshal(screening)
	if err != nil {
		t.Fatal(err)
	}
	screeningHash := artifactHash(screeningBytes)
	artifact := boundaryResolutionArtifact{
		SchemaVersion: "slope-resolution-v1", ProgramPolicyVersion: DemoProgramPolicyVersion, ID: evidenceID, EvidenceType: EvidenceTypeBoundaryExclusion,
		FieldID: field.ID, BoundaryVersion: 4, CRS: "EPSG:4326",
		ParentBoundaryEvidenceRecordID: parentID, ParentBoundaryArtifactHash: parentHash, ParentBoundaryVersion: 3,
		SourceEvidenceRecordID: sourceID, SourceArtifactHash: sourceHash,
		RevisedScreeningEvidenceRecordID: screeningID, RevisedScreeningArtifactHash: screeningHash,
		IssuingRole: DemoBoundaryIssuerRole, RecordedAt: time.Date(2026, time.August, 20, 7, 58, 0, 0, time.UTC),
		ExpiresAt: time.Date(2027, time.August, 20, 7, 58, 0, 0, time.UTC), ConfirmationStatus: "CONFIRMED",
		ReviewerRole: DemoPlacementReviewerRole, ReviewerAuthorizationRecordID: "a4c0b3a1-6f74-4cad-9df1-7d7bc68d1002",
		Boundary: polygonGeometry{Type: "Polygon", Coordinates: [][][]float64{{{-84.5585, 42.7315}, {-84.5545, 42.7315}, {-84.5545, 42.735036}, {-84.5585, 42.735036}, {-84.5585, 42.7315}}}},
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	hash := artifactHash(encoded)
	authorization := reviewerAuthorizationArtifact{
		SchemaVersion: "review-authorization-v1", ProgramPolicyVersion: DemoProgramPolicyVersion, ID: artifact.ReviewerAuthorizationRecordID,
		EvidenceRecordID: evidenceID, EvidenceArtifactHash: hash, FieldID: field.ID, BoundaryVersion: 4,
		ReviewerRole: DemoPlacementReviewerRole, ApprovalStatus: "APPROVED",
		RecordedAt: time.Date(2026, time.August, 20, 7, 59, 0, 0, time.UTC),
	}
	authorizationBytes, err := json.Marshal(authorization)
	if err != nil {
		t.Fatal(err)
	}
	authorizationHash := artifactHash(authorizationBytes)
	field.BoundaryVersion = 4
	field.SlopeResolution = &SlopeResolutionReference{
		ProgramPolicyVersion: DemoProgramPolicyVersion, EvidenceRecordID: evidenceID, EvidenceType: EvidenceTypeBoundaryExclusion, ArtifactHash: hash,
		BoundaryVersion: 4, ParentBoundaryEvidenceRecordID: parentID, ParentBoundaryArtifactHash: parentHash, ParentBoundaryVersion: 3,
		SourceEvidenceRecordID: sourceID, SourceArtifactHash: sourceHash,
		RevisedScreeningEvidenceRecordID: screeningID, RevisedScreeningArtifactHash: screeningHash,
	}
	store := testEvidenceStore{
		evidenceID:                             {ID: evidenceID, ArtifactHash: hash, Artifact: encoded},
		sourceID:                               {ID: sourceID, ArtifactHash: sourceHash, Artifact: source},
		parentID:                               {ID: parentID, ArtifactHash: parentHash, Artifact: parentBytes},
		screeningID:                            {ID: screeningID, ArtifactHash: screeningHash, Artifact: screeningBytes},
		artifact.ReviewerAuthorizationRecordID: {ID: artifact.ReviewerAuthorizationRecordID, ArtifactHash: authorizationHash, Artifact: authorizationBytes},
	}
	return field, store, time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
}

func updateBoundaryArtifact(t *testing.T, field *FieldInput, store testEvidenceStore, update func(*boundaryResolutionArtifact)) {
	t.Helper()
	record := store[field.SlopeResolution.EvidenceRecordID]
	var artifact boundaryResolutionArtifact
	if err := json.Unmarshal(record.Artifact, &artifact); err != nil {
		t.Fatal(err)
	}
	update(&artifact)
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	hash := artifactHash(encoded)
	record.Artifact, record.ArtifactHash = encoded, hash
	store[record.ID] = record
	field.SlopeResolution.ArtifactHash = hash
	authorizationRecord := store[artifact.ReviewerAuthorizationRecordID]
	if len(authorizationRecord.Artifact) > 0 {
		var authorization reviewerAuthorizationArtifact
		if err := json.Unmarshal(authorizationRecord.Artifact, &authorization); err != nil {
			t.Fatal(err)
		}
		authorization.EvidenceArtifactHash = hash
		authorizationBytes, err := json.Marshal(authorization)
		if err != nil {
			t.Fatal(err)
		}
		authorizationRecord.Artifact = authorizationBytes
		authorizationRecord.ArtifactHash = artifactHash(authorizationBytes)
		store[authorizationRecord.ID] = authorizationRecord
	}
}

func authorizationID(t *testing.T, field *FieldInput, store testEvidenceStore) string {
	t.Helper()
	record := store[field.SlopeResolution.EvidenceRecordID]
	var artifact boundaryResolutionArtifact
	if err := json.Unmarshal(record.Artifact, &artifact); err != nil {
		t.Fatal(err)
	}
	return artifact.ReviewerAuthorizationRecordID
}

func updateAuthorizationArtifact(t *testing.T, field *FieldInput, store testEvidenceStore, update func(*reviewerAuthorizationArtifact)) {
	t.Helper()
	id := authorizationID(t, field, store)
	record := store[id]
	var artifact reviewerAuthorizationArtifact
	if err := json.Unmarshal(record.Artifact, &artifact); err != nil {
		t.Fatal(err)
	}
	update(&artifact)
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	record.Artifact = encoded
	record.ArtifactHash = artifactHash(encoded)
	store[id] = record
}

func updateScreeningArtifact(t *testing.T, field *FieldInput, store testEvidenceStore, updateResponseHash bool, update func(*revisedScreeningArtifact)) {
	t.Helper()
	id := field.SlopeResolution.RevisedScreeningEvidenceRecordID
	record := store[id]
	var screening revisedScreeningArtifact
	if err := json.Unmarshal(record.Artifact, &screening); err != nil {
		t.Fatal(err)
	}
	update(&screening)
	if updateResponseHash {
		hash, err := compactArtifactHash(screening.Response)
		if err != nil {
			t.Fatal(err)
		}
		screening.ResponseHash = hash
	}
	encoded, err := json.Marshal(screening)
	if err != nil {
		t.Fatal(err)
	}
	hash := artifactHash(encoded)
	record.Artifact, record.ArtifactHash = encoded, hash
	store[id] = record
	field.SlopeResolution.RevisedScreeningArtifactHash = hash
	updateBoundaryArtifact(t, field, store, func(artifact *boundaryResolutionArtifact) { artifact.RevisedScreeningArtifactHash = hash })
}
