package placement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/salarkhannn/pfas-load-control/internal/mireye"
)

const EvidenceTypeBoundaryExclusion = "CONFIRMED_BOUNDARY_EXCLUSION"

type EvidenceArtifact struct {
	ID           string
	ArtifactHash string
	Artifact     []byte
}

type SlopeEvidenceStore interface {
	LoadSlopeEvidence(context.Context, string) (EvidenceArtifact, error)
}

type ResolutionVerification struct {
	ProgramPolicyVersion             string                       `json:"programPolicyVersion"`
	DemonstrationRoles               bool                         `json:"demonstrationRoles"`
	EvidenceRecordID                 string                       `json:"evidenceRecordId"`
	EvidenceType                     string                       `json:"evidenceType"`
	ArtifactHash                     string                       `json:"artifactHash"`
	FieldID                          string                       `json:"fieldId"`
	BoundaryVersion                  int                          `json:"boundaryVersion"`
	CRS                              string                       `json:"crs"`
	ParentBoundaryEvidenceRecordID   string                       `json:"parentBoundaryEvidenceRecordId"`
	ParentBoundaryArtifactHash       string                       `json:"parentBoundaryArtifactHash"`
	ParentBoundaryVersion            int                          `json:"parentBoundaryVersion"`
	SourceEvidenceRecordID           string                       `json:"sourceEvidenceRecordId"`
	SourceArtifactHash               string                       `json:"sourceArtifactHash"`
	RevisedScreeningEvidenceRecordID string                       `json:"revisedScreeningEvidenceRecordId"`
	RevisedScreeningArtifactHash     string                       `json:"revisedScreeningArtifactHash"`
	IssuingRole                      string                       `json:"issuingRole"`
	ReviewerRole                     string                       `json:"reviewerRole"`
	ReviewerAuthorizationRecordID    string                       `json:"reviewerAuthorizationRecordId"`
	ReviewerAuthorizationHash        string                       `json:"reviewerAuthorizationHash"`
	RecordedAt                       time.Time                    `json:"recordedAt"`
	VerifiedAt                       time.Time                    `json:"verifiedAt"`
	DerivedUsableAcres               string                       `json:"derivedUsableAcres"`
	HighSlopeSamplesExcluded         int                          `json:"highSlopeSamplesExcluded"`
	ParentBoundaryAcres              string                       `json:"parentBoundaryAcres"`
	RevisedScreening                 RevisedScreeningVerification `json:"revisedScreening"`
	SlopeConversion                  SlopeConversion              `json:"slopeConversion"`
}

type RevisedScreeningVerification struct {
	EvidenceRecordID         string    `json:"evidenceRecordId"`
	ArtifactHash             string    `json:"artifactHash"`
	Endpoint                 string    `json:"endpoint"`
	RequestID                string    `json:"requestId"`
	RequestHash              string    `json:"requestHash"`
	ResponseHash             string    `json:"responseHash"`
	RetrievedAt              time.Time `json:"retrievedAt"`
	AlgorithmVersion         string    `json:"algorithmVersion"`
	MinimumSpacingMeters     string    `json:"minimumSpacingMeters"`
	MaxRequestSamples        int       `json:"maxRequestSamples"`
	RequestedSampleCount     int       `json:"requestedSampleCount"`
	ReturnedSampleCount      int       `json:"returnedSampleCount"`
	BoundaryNearSampleCount  int       `json:"boundaryNearSampleCount"`
	InteriorSampleCount      int       `json:"interiorSampleCount"`
	MaximumSlopeDegrees      string    `json:"maximumSlopeDegrees"`
	MaximumSlopeGradePercent string    `json:"maximumSlopeGradePercent"`
	Status                   string    `json:"status"`
	Limitation               string    `json:"limitation"`
}

type SlopeConversion struct {
	OriginalDegrees       string `json:"originalDegrees"`
	DerivedGradePercent   string `json:"derivedGradePercent"`
	ThresholdGradePercent string `json:"thresholdGradePercent"`
	ThresholdDegrees      string `json:"thresholdDegrees"`
	Formula               string `json:"formula"`
	PolicySourceURL       string `json:"policySourceUrl"`
}

type ResolutionError struct {
	Code string
	err  error
}

func (e *ResolutionError) Error() string { return e.Code + ": " + e.err.Error() }
func (e *ResolutionError) Unwrap() error { return e.err }

type boundaryResolutionArtifact struct {
	SchemaVersion                    string          `json:"schemaVersion"`
	ProgramPolicyVersion             string          `json:"programPolicyVersion"`
	ID                               string          `json:"id"`
	EvidenceType                     string          `json:"evidenceType"`
	FieldID                          string          `json:"fieldId"`
	BoundaryVersion                  int             `json:"boundaryVersion"`
	CRS                              string          `json:"crs"`
	ParentBoundaryEvidenceRecordID   string          `json:"parentBoundaryEvidenceRecordId"`
	ParentBoundaryArtifactHash       string          `json:"parentBoundaryArtifactHash"`
	ParentBoundaryVersion            int             `json:"parentBoundaryVersion"`
	SourceEvidenceRecordID           string          `json:"sourceEvidenceRecordId"`
	SourceArtifactHash               string          `json:"sourceArtifactHash"`
	RevisedScreeningEvidenceRecordID string          `json:"revisedScreeningEvidenceRecordId"`
	RevisedScreeningArtifactHash     string          `json:"revisedScreeningArtifactHash"`
	IssuingRole                      string          `json:"issuingRole"`
	RecordedAt                       time.Time       `json:"recordedAt"`
	ExpiresAt                        time.Time       `json:"expiresAt"`
	ConfirmationStatus               string          `json:"confirmationStatus"`
	ReviewerRole                     string          `json:"reviewerRole"`
	ReviewerAuthorizationRecordID    string          `json:"reviewerAuthorizationRecordId"`
	SupersededBy                     string          `json:"supersededBy,omitempty"`
	Boundary                         polygonGeometry `json:"boundary"`
}

type parentBoundaryArtifact struct {
	SchemaVersion      string          `json:"schemaVersion"`
	ID                 string          `json:"id"`
	FieldID            string          `json:"fieldId"`
	BoundaryVersion    int             `json:"boundaryVersion"`
	CRS                string          `json:"crs"`
	ConfirmationStatus string          `json:"confirmationStatus"`
	RecordedAt         time.Time       `json:"recordedAt"`
	Boundary           polygonGeometry `json:"boundary"`
}

type revisedScreeningArtifact struct {
	SchemaVersion         string          `json:"schemaVersion"`
	ID                    string          `json:"id"`
	FieldID               string          `json:"fieldId"`
	BoundaryVersion       int             `json:"boundaryVersion"`
	Endpoint              string          `json:"endpoint"`
	RequestID             string          `json:"requestId"`
	HTTPStatus            int             `json:"httpStatus"`
	RetrievedAt           time.Time       `json:"retrievedAt"`
	RequestHash           string          `json:"requestHash"`
	ResponseHash          string          `json:"responseHash"`
	AlgorithmVersion      string          `json:"algorithmVersion"`
	MinimumSpacingMeters  float64         `json:"minimumSpacingMeters"`
	BoundaryNearDistanceM float64         `json:"boundaryNearDistanceMeters"`
	MaxRequestSamples     int             `json:"maxRequestSamples"`
	RequestedSampleCount  int             `json:"requestedSampleCount"`
	ReturnedSampleCount   int             `json:"returnedSampleCount"`
	Request               json.RawMessage `json:"request"`
	Response              json.RawMessage `json:"response"`
}

type reviewerAuthorizationArtifact struct {
	SchemaVersion        string    `json:"schemaVersion"`
	ProgramPolicyVersion string    `json:"programPolicyVersion"`
	ID                   string    `json:"id"`
	EvidenceRecordID     string    `json:"evidenceRecordId"`
	EvidenceArtifactHash string    `json:"evidenceArtifactHash"`
	FieldID              string    `json:"fieldId"`
	BoundaryVersion      int       `json:"boundaryVersion"`
	ReviewerRole         string    `json:"reviewerRole"`
	ApprovalStatus       string    `json:"approvalStatus"`
	RecordedAt           time.Time `json:"recordedAt"`
	SupersededBy         string    `json:"supersededBy,omitempty"`
}

func CanonicalizeSlopeResolutionArtifact(raw []byte) ([]byte, string, error) {
	var artifact boundaryResolutionArtifact
	if err := decodeStrict(raw, &artifact); err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return nil, "", err
	}
	return encoded, artifactHash(encoded), nil
}

func CanonicalizeReviewerAuthorizationArtifact(raw []byte) ([]byte, string, error) {
	var artifact reviewerAuthorizationArtifact
	if err := decodeStrict(raw, &artifact); err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return nil, "", err
	}
	return encoded, artifactHash(encoded), nil
}

func CanonicalizeParentBoundaryArtifact(raw []byte) ([]byte, string, error) {
	var artifact parentBoundaryArtifact
	if err := decodeStrict(raw, &artifact); err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return nil, "", err
	}
	return encoded, artifactHash(encoded), nil
}

func CanonicalizeRevisedScreeningArtifact(raw []byte) ([]byte, string, error) {
	var artifact revisedScreeningArtifact
	if err := decodeStrict(raw, &artifact); err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return nil, "", err
	}
	return encoded, artifactHash(encoded), nil
}

type polygonGeometry struct {
	Type        string        `json:"type"`
	Coordinates [][][]float64 `json:"coordinates"`
}

func VerifySlopeResolution(ctx context.Context, field FieldInput, store SlopeEvidenceStore, asOf time.Time) (ResolutionVerification, error) {
	return VerifySlopeResolutionWithPolicy(ctx, field, store, asOf, CurrentResolutionPolicy())
}

func VerifySlopeResolutionWithPolicy(ctx context.Context, field FieldInput, store SlopeEvidenceStore, asOf time.Time, policy ResolutionPolicy) (ResolutionVerification, error) {
	if field.SlopeResolution == nil {
		return ResolutionVerification{}, resolutionError("MISSING_REFERENCE", errors.New("slope resolution evidence is required"))
	}
	if store == nil {
		return ResolutionVerification{}, resolutionError("UNKNOWN_EVIDENCE_ID", errors.New("no immutable evidence store is configured"))
	}
	ref := field.SlopeResolution
	if _, err := uuid.Parse(ref.EvidenceRecordID); err != nil {
		return ResolutionVerification{}, resolutionError("UNKNOWN_EVIDENCE_ID", errors.New("evidence record ID is invalid"))
	}
	record, err := store.LoadSlopeEvidence(ctx, ref.EvidenceRecordID)
	if err != nil {
		return ResolutionVerification{}, resolutionError("UNKNOWN_EVIDENCE_ID", err)
	}
	if record.ID != ref.EvidenceRecordID {
		return ResolutionVerification{}, resolutionError("EVIDENCE_ID_MISMATCH", errors.New("stored evidence ID does not match the reference"))
	}
	actualHash := artifactHash(record.Artifact)
	if record.ArtifactHash != actualHash || ref.ArtifactHash != actualHash {
		return ResolutionVerification{}, resolutionError("ARTIFACT_HASH_MISMATCH", errors.New("stored evidence bytes do not match the referenced hash"))
	}

	var artifact boundaryResolutionArtifact
	if err := decodeStrict(record.Artifact, &artifact); err != nil {
		return ResolutionVerification{}, resolutionError("INVALID_EVIDENCE_ARTIFACT", err)
	}
	if artifact.ID != ref.EvidenceRecordID {
		return ResolutionVerification{}, resolutionError("EVIDENCE_ID_MISMATCH", errors.New("artifact ID does not match the reference"))
	}
	if policy.Version == "" || ref.ProgramPolicyVersion != policy.Version || artifact.ProgramPolicyVersion != policy.Version {
		return ResolutionVerification{}, resolutionError("PROGRAM_POLICY_MISMATCH", errors.New("resolution evidence does not match the active versioned program policy"))
	}
	if ref.EvidenceType != EvidenceTypeBoundaryExclusion || artifact.EvidenceType != ref.EvidenceType {
		return ResolutionVerification{}, resolutionError("UNSUPPORTED_EVIDENCE_TYPE", errors.New("evidence type cannot resolve the sampled slope"))
	}
	if artifact.FieldID != field.ID {
		return ResolutionVerification{}, resolutionError("FIELD_MISMATCH", errors.New("evidence belongs to another field"))
	}
	if field.BoundaryVersion < 1 || ref.BoundaryVersion != field.BoundaryVersion || artifact.BoundaryVersion != field.BoundaryVersion {
		return ResolutionVerification{}, resolutionError("BOUNDARY_VERSION_MISMATCH", errors.New("evidence does not match the field boundary version"))
	}
	if artifact.CRS != "EPSG:4326" {
		return ResolutionVerification{}, resolutionError("UNSUPPORTED_CRS", errors.New("revised boundary must use EPSG:4326"))
	}
	if artifact.ParentBoundaryEvidenceRecordID != ref.ParentBoundaryEvidenceRecordID || artifact.ParentBoundaryArtifactHash != ref.ParentBoundaryArtifactHash || artifact.ParentBoundaryVersion != ref.ParentBoundaryVersion {
		return ResolutionVerification{}, resolutionError("PARENT_BOUNDARY_REFERENCE_MISMATCH", errors.New("parent boundary reference does not match the resolution artifact"))
	}
	if artifact.RevisedScreeningEvidenceRecordID != ref.RevisedScreeningEvidenceRecordID || artifact.RevisedScreeningArtifactHash != ref.RevisedScreeningArtifactHash {
		return ResolutionVerification{}, resolutionError("REVISED_SCREENING_REFERENCE_MISMATCH", errors.New("revised screening reference does not match the resolution artifact"))
	}
	if artifact.ConfirmationStatus != "CONFIRMED" {
		return ResolutionVerification{}, resolutionError("EVIDENCE_NOT_CONFIRMED", errors.New("evidence has not been confirmed"))
	}
	if !configuredRole(policy.AuthorizedIssuerRoles, artifact.IssuingRole) {
		return ResolutionVerification{}, resolutionError("UNAUTHORIZED_ISSUER", errors.New("issuing role is not authorized by the active program policy"))
	}
	if !configuredRole(policy.AuthorizedReviewerRoles, artifact.ReviewerRole) || artifact.ReviewerRole == artifact.IssuingRole {
		return ResolutionVerification{}, resolutionError("UNAUTHORIZED_REVIEWER", errors.New("a separately configured reviewer must approve the evidence"))
	}
	if _, err := uuid.Parse(artifact.ReviewerAuthorizationRecordID); err != nil {
		return ResolutionVerification{}, resolutionError("MISSING_REVIEW_AUTHORIZATION", errors.New("review authorization record is invalid"))
	}
	if artifact.RecordedAt.IsZero() || artifact.RecordedAt.After(asOf) {
		return ResolutionVerification{}, resolutionError("INVALID_RECORDED_TIME", errors.New("evidence timestamp is missing or later than the decision"))
	}
	if artifact.ExpiresAt.IsZero() || !artifact.ExpiresAt.After(asOf) {
		return ResolutionVerification{}, resolutionError("EVIDENCE_EXPIRED", errors.New("evidence expired before the decision"))
	}
	if strings.TrimSpace(artifact.SupersededBy) != "" {
		return ResolutionVerification{}, resolutionError("EVIDENCE_SUPERSEDED", errors.New("evidence has been superseded"))
	}
	if artifact.SourceEvidenceRecordID != ref.SourceEvidenceRecordID || artifact.SourceArtifactHash != ref.SourceArtifactHash {
		return ResolutionVerification{}, resolutionError("SOURCE_REFERENCE_MISMATCH", errors.New("source evidence reference does not match the artifact"))
	}

	revisedRing, derivedAcres, err := validateBoundaryGeometry(artifact.Boundary)
	if err != nil {
		return ResolutionVerification{}, resolutionError("INVALID_BOUNDARY_GEOMETRY", err)
	}
	parentRecord, err := store.LoadSlopeEvidence(ctx, ref.ParentBoundaryEvidenceRecordID)
	if err != nil || parentRecord.ID != ref.ParentBoundaryEvidenceRecordID {
		return ResolutionVerification{}, resolutionError("UNKNOWN_PARENT_BOUNDARY", errors.New("parent boundary evidence was not found"))
	}
	parentHash := artifactHash(parentRecord.Artifact)
	if parentRecord.ArtifactHash != parentHash || ref.ParentBoundaryArtifactHash != parentHash {
		return ResolutionVerification{}, resolutionError("PARENT_BOUNDARY_HASH_MISMATCH", errors.New("parent boundary bytes do not match the referenced hash"))
	}
	var parent parentBoundaryArtifact
	if err := decodeStrict(parentRecord.Artifact, &parent); err != nil {
		return ResolutionVerification{}, resolutionError("INVALID_PARENT_BOUNDARY", err)
	}
	if parent.ID != parentRecord.ID || parent.FieldID != field.ID {
		return ResolutionVerification{}, resolutionError("PARENT_BOUNDARY_FIELD_MISMATCH", errors.New("parent boundary belongs to another field"))
	}
	if parent.BoundaryVersion != ref.ParentBoundaryVersion || artifact.BoundaryVersion != parent.BoundaryVersion+1 {
		return ResolutionVerification{}, resolutionError("PARENT_BOUNDARY_VERSION_MISMATCH", errors.New("revised boundary does not immediately follow the parent version"))
	}
	if parent.CRS != "EPSG:4326" || parent.CRS != artifact.CRS {
		return ResolutionVerification{}, resolutionError("PARENT_BOUNDARY_CRS_MISMATCH", errors.New("parent and revised boundaries must use EPSG:4326"))
	}
	if parent.ConfirmationStatus != "CONFIRMED" || parent.RecordedAt.IsZero() || parent.RecordedAt.After(artifact.RecordedAt) {
		return ResolutionVerification{}, resolutionError("PARENT_BOUNDARY_NOT_CONFIRMED", errors.New("parent boundary was not confirmed before the revision"))
	}
	parentRing, parentAcres, err := validateBoundaryGeometry(parent.Boundary)
	if err != nil {
		return ResolutionVerification{}, resolutionError("INVALID_PARENT_BOUNDARY", err)
	}
	if !polygonContainsRing(parentRing, revisedRing) {
		return ResolutionVerification{}, resolutionError("REVISED_BOUNDARY_OUTSIDE_PARENT", errors.New("revised boundary is not fully contained by its confirmed parent"))
	}

	source, err := store.LoadSlopeEvidence(ctx, ref.SourceEvidenceRecordID)
	if err != nil || source.ID != ref.SourceEvidenceRecordID {
		return ResolutionVerification{}, resolutionError("UNKNOWN_SOURCE_EVIDENCE_ID", errors.New("source slope evidence was not found"))
	}
	if actual := artifactHash(source.Artifact); source.ArtifactHash != actual || ref.SourceArtifactHash != actual {
		return ResolutionVerification{}, resolutionError("SOURCE_HASH_MISMATCH", errors.New("source slope evidence bytes do not match the referenced hash"))
	}
	highSamples, err := highSlopeSamples(source.Artifact)
	if err != nil {
		return ResolutionVerification{}, resolutionError("INVALID_SOURCE_EVIDENCE", err)
	}
	if len(highSamples) == 0 {
		return ResolutionVerification{}, resolutionError("NO_HIGH_SLOPE_SAMPLE", errors.New("source evidence contains no excessive-slope sample to resolve"))
	}
	for _, sample := range highSamples {
		if pointInPolygonOrBoundary(sample.Longitude, sample.Latitude, revisedRing) {
			return ResolutionVerification{}, resolutionError("HIGH_SLOPE_STILL_INSIDE_BOUNDARY", fmt.Errorf("sample %d remains inside the revised boundary", sample.Index))
		}
	}
	revisedScreening, err := verifyRevisedSlopeScreening(ctx, field, store, artifact, revisedRing, asOf, policy)
	if err != nil {
		return ResolutionVerification{}, err
	}
	authorization, err := store.LoadSlopeEvidence(ctx, artifact.ReviewerAuthorizationRecordID)
	if err != nil || authorization.ID != artifact.ReviewerAuthorizationRecordID {
		return ResolutionVerification{}, resolutionError("UNKNOWN_REVIEW_AUTHORIZATION", errors.New("review authorization record was not found"))
	}
	authorizationHash := artifactHash(authorization.Artifact)
	if authorization.ArtifactHash != authorizationHash {
		return ResolutionVerification{}, resolutionError("REVIEW_AUTHORIZATION_HASH_MISMATCH", errors.New("review authorization bytes do not match the stored hash"))
	}
	var approval reviewerAuthorizationArtifact
	if err := decodeStrict(authorization.Artifact, &approval); err != nil {
		return ResolutionVerification{}, resolutionError("INVALID_REVIEW_AUTHORIZATION", err)
	}
	if approval.ID != authorization.ID || approval.ProgramPolicyVersion != policy.Version || approval.EvidenceRecordID != artifact.ID || approval.EvidenceArtifactHash != actualHash {
		return ResolutionVerification{}, resolutionError("REVIEW_AUTHORIZATION_EVIDENCE_MISMATCH", errors.New("review authorization does not approve this immutable evidence artifact"))
	}
	if approval.FieldID != field.ID || approval.BoundaryVersion != field.BoundaryVersion {
		return ResolutionVerification{}, resolutionError("REVIEW_AUTHORIZATION_SCOPE_MISMATCH", errors.New("review authorization belongs to another field or boundary version"))
	}
	if approval.ReviewerRole != artifact.ReviewerRole || !configuredRole(policy.AuthorizedReviewerRoles, approval.ReviewerRole) || approval.ReviewerRole == artifact.IssuingRole || approval.ApprovalStatus != "APPROVED" {
		return ResolutionVerification{}, resolutionError("UNAUTHORIZED_APPROVAL", errors.New("review authorization was not approved by a separately configured reviewer"))
	}
	if approval.RecordedAt.IsZero() || approval.RecordedAt.Before(artifact.RecordedAt) || approval.RecordedAt.After(asOf) {
		return ResolutionVerification{}, resolutionError("INVALID_REVIEW_AUTHORIZATION_TIME", errors.New("review authorization timestamp is outside the valid decision window"))
	}
	if strings.TrimSpace(approval.SupersededBy) != "" {
		return ResolutionVerification{}, resolutionError("REVIEW_AUTHORIZATION_SUPERSEDED", errors.New("review authorization has been superseded"))
	}

	maximumOriginalSlope, err := maximumSlopeDegrees(source.Artifact)
	if err != nil {
		return ResolutionVerification{}, resolutionError("INVALID_SOURCE_EVIDENCE", err)
	}

	return ResolutionVerification{
		ProgramPolicyVersion: policy.Version, DemonstrationRoles: policy.DemonstrationRoles,
		EvidenceRecordID: ref.EvidenceRecordID, EvidenceType: artifact.EvidenceType,
		ArtifactHash: actualHash, FieldID: artifact.FieldID, BoundaryVersion: artifact.BoundaryVersion,
		CRS: artifact.CRS, ParentBoundaryEvidenceRecordID: parent.ID,
		ParentBoundaryArtifactHash: parentHash, ParentBoundaryVersion: parent.BoundaryVersion,
		SourceEvidenceRecordID: artifact.SourceEvidenceRecordID, SourceArtifactHash: artifact.SourceArtifactHash,
		RevisedScreeningEvidenceRecordID: artifact.RevisedScreeningEvidenceRecordID,
		RevisedScreeningArtifactHash:     artifact.RevisedScreeningArtifactHash,
		IssuingRole:                      artifact.IssuingRole, ReviewerRole: artifact.ReviewerRole,
		ReviewerAuthorizationRecordID: artifact.ReviewerAuthorizationRecordID,
		ReviewerAuthorizationHash:     authorizationHash,
		RecordedAt:                    artifact.RecordedAt, VerifiedAt: asOf, DerivedUsableAcres: derivedAcres,
		HighSlopeSamplesExcluded: len(highSamples), ParentBoundaryAcres: parentAcres,
		RevisedScreening: revisedScreening, SlopeConversion: SlopeConversionForDegrees(maximumOriginalSlope),
	}, nil
}

func VerifyRevisedSlopeScreening(ctx context.Context, field FieldInput, store SlopeEvidenceStore, asOf time.Time) (RevisedScreeningVerification, error) {
	return VerifyRevisedSlopeScreeningWithPolicy(ctx, field, store, asOf, CurrentResolutionPolicy())
}

func VerifyRevisedSlopeScreeningWithPolicy(ctx context.Context, field FieldInput, store SlopeEvidenceStore, asOf time.Time, policy ResolutionPolicy) (RevisedScreeningVerification, error) {
	if field.SlopeResolution == nil || store == nil {
		return RevisedScreeningVerification{}, resolutionError("MISSING_REFERENCE", errors.New("slope resolution evidence is required"))
	}
	record, err := store.LoadSlopeEvidence(ctx, field.SlopeResolution.EvidenceRecordID)
	if err != nil {
		return RevisedScreeningVerification{}, resolutionError("UNKNOWN_EVIDENCE_ID", err)
	}
	if artifactHash(record.Artifact) != field.SlopeResolution.ArtifactHash || record.ArtifactHash != field.SlopeResolution.ArtifactHash {
		return RevisedScreeningVerification{}, resolutionError("ARTIFACT_HASH_MISMATCH", errors.New("resolution artifact hash does not match"))
	}
	var artifact boundaryResolutionArtifact
	if err := decodeStrict(record.Artifact, &artifact); err != nil {
		return RevisedScreeningVerification{}, resolutionError("INVALID_EVIDENCE_ARTIFACT", err)
	}
	if policy.Version == "" || field.SlopeResolution.ProgramPolicyVersion != policy.Version || artifact.ProgramPolicyVersion != policy.Version {
		return RevisedScreeningVerification{}, resolutionError("PROGRAM_POLICY_MISMATCH", errors.New("screening evidence does not match the active versioned program policy"))
	}
	ring, _, err := validateBoundaryGeometry(artifact.Boundary)
	if err != nil {
		return RevisedScreeningVerification{}, resolutionError("INVALID_BOUNDARY_GEOMETRY", err)
	}
	return verifyRevisedSlopeScreening(ctx, field, store, artifact, ring, asOf, policy)
}

func verifyRevisedSlopeScreening(ctx context.Context, field FieldInput, store SlopeEvidenceStore, resolution boundaryResolutionArtifact, revisedRing [][]float64, asOf time.Time, policy ResolutionPolicy) (RevisedScreeningVerification, error) {
	record, err := store.LoadSlopeEvidence(ctx, resolution.RevisedScreeningEvidenceRecordID)
	if err != nil || record.ID != resolution.RevisedScreeningEvidenceRecordID {
		return RevisedScreeningVerification{}, resolutionError("UNKNOWN_REVISED_SCREENING", errors.New("revised-boundary Mireye screening was not found"))
	}
	actualHash := artifactHash(record.Artifact)
	if record.ArtifactHash != actualHash || resolution.RevisedScreeningArtifactHash != actualHash || field.SlopeResolution.RevisedScreeningArtifactHash != actualHash {
		return RevisedScreeningVerification{}, resolutionError("REVISED_SCREENING_HASH_MISMATCH", errors.New("revised screening bytes do not match the referenced hash"))
	}
	var screening revisedScreeningArtifact
	if err := decodeStrict(record.Artifact, &screening); err != nil {
		return RevisedScreeningVerification{}, resolutionError("INVALID_REVISED_SCREENING", err)
	}
	if screening.ID != record.ID || screening.FieldID != field.ID || screening.BoundaryVersion != field.BoundaryVersion {
		return RevisedScreeningVerification{}, resolutionError("REVISED_SCREENING_SCOPE_MISMATCH", errors.New("revised screening belongs to another field or boundary version"))
	}
	if screening.RetrievedAt.IsZero() || screening.RetrievedAt.After(asOf) {
		return RevisedScreeningVerification{}, resolutionError("INVALID_REVISED_SCREENING_TIME", errors.New("revised screening timestamp is missing or later than the decision"))
	}
	if policy.Sampling.TargetSampleCount > policy.Sampling.MaxRequestSamples {
		return RevisedScreeningVerification{}, resolutionError("SAMPLING_LIMIT_REQUIRES_REVIEW", errors.New("the configured sampling target exceeds the bounded Mireye request limit"))
	}
	if screening.AlgorithmVersion != policy.Sampling.AlgorithmVersion || screening.MinimumSpacingMeters != policy.Sampling.MinimumSpacingMeters || screening.BoundaryNearDistanceM != policy.Sampling.BoundaryNearDistanceMeters || screening.MaxRequestSamples != policy.Sampling.MaxRequestSamples || screening.RequestedSampleCount != policy.Sampling.TargetSampleCount {
		return RevisedScreeningVerification{}, resolutionError("UNSUPPORTED_SAMPLING_RULE", errors.New("revised screening does not match the active bounded sampling policy"))
	}
	if screening.RequestedSampleCount > screening.MaxRequestSamples {
		return RevisedScreeningVerification{}, resolutionError("SAMPLING_LIMIT_REQUIRES_REVIEW", errors.New("the configured sampling target exceeds the bounded Mireye request limit"))
	}
	requestHash, requestHashErr := compactArtifactHash(screening.Request)
	responseHash, responseHashErr := compactArtifactHash(screening.Response)
	if requestHashErr != nil || responseHashErr != nil || requestHash != screening.RequestHash || responseHash != screening.ResponseHash {
		return RevisedScreeningVerification{}, resolutionError("REVISED_SCREENING_HASH_MISMATCH", errors.New("stored Mireye request or response bytes do not match their capture hashes"))
	}
	plan, err := planPolygonScreeningSamples(revisedRing, policy.Sampling)
	if err != nil {
		if errors.Is(err, errSamplingLimitExceeded) {
			return RevisedScreeningVerification{}, resolutionError("SAMPLING_LIMIT_REQUIRES_REVIEW", err)
		}
		return RevisedScreeningVerification{}, resolutionError("INADEQUATE_REVISED_SCREENING", err)
	}
	replayed, err := mireye.ReplayFetchBatch(mireye.FetchBatchCapture{
		SourceURL: screening.Endpoint, RequestID: screening.RequestID, HTTPStatus: screening.HTTPStatus,
		FetchedAt: screening.RetrievedAt, ExpectedRequestHash: screening.RequestHash,
		ExpectedResponseHash: screening.ResponseHash, Request: screening.Request, Response: screening.Response,
	})
	if err != nil {
		return RevisedScreeningVerification{}, resolutionError("INADEQUATE_REVISED_SCREENING", err)
	}
	var request mireye.FetchBatchRequest
	if err := json.Unmarshal(replayed.Request, &request); err != nil {
		return RevisedScreeningVerification{}, resolutionError("INVALID_REVISED_SCREENING", err)
	}
	if len(request.Fields) != 1 || request.Fields[0] != "slope_degrees" || len(request.Locations) != len(plan.Locations) || len(replayed.Results) != len(plan.Locations) || screening.ReturnedSampleCount != len(replayed.Results) {
		return RevisedScreeningVerification{}, resolutionError("INADEQUATE_REVISED_SCREENING", errors.New("screening did not return slope for every requested sample"))
	}
	maximum := -1.0
	for index, expectedPoint := range plan.Locations {
		location := request.Locations[index]
		result := replayed.Results[index]
		if !sameCoordinate(location.Longitude, expectedPoint[0]) || !sameCoordinate(location.Latitude, expectedPoint[1]) || result.Latitude == nil || result.Longitude == nil || !sameCoordinate(*result.Longitude, expectedPoint[0]) || !sameCoordinate(*result.Latitude, expectedPoint[1]) || !pointInPolygonOrBoundary(expectedPoint[0], expectedPoint[1], revisedRing) {
			return RevisedScreeningVerification{}, resolutionError("INADEQUATE_REVISED_SCREENING", fmt.Errorf("sample %d does not match the deterministic polygon screening plan", index))
		}
		fact, ok := result.Fields["slope_degrees"]
		if !result.OK || !ok || fact.Status != "ok" || fact.Unit == nil || *fact.Unit != "degrees" {
			return RevisedScreeningVerification{}, resolutionError("INADEQUATE_REVISED_SCREENING", fmt.Errorf("sample %d has no usable slope result", index))
		}
		var slope float64
		if err := json.Unmarshal(fact.Value, &slope); err != nil || math.IsNaN(slope) || math.IsInf(slope, 0) {
			return RevisedScreeningVerification{}, resolutionError("INVALID_REVISED_SCREENING", fmt.Errorf("sample %d slope is invalid", index))
		}
		maximum = math.Max(maximum, slope)
	}
	if maximum < 0 {
		return RevisedScreeningVerification{}, resolutionError("INADEQUATE_REVISED_SCREENING", errors.New("revised boundary has no supporting slope samples"))
	}
	if SlopeExceedsSurfaceLimit(maximum) {
		return RevisedScreeningVerification{}, resolutionError("REVISED_BOUNDARY_SLOPE_EXCEEDED", fmt.Errorf("revised boundary maximum slope %.6f degrees exceeds the six-percent grade threshold", maximum))
	}
	return RevisedScreeningVerification{
		EvidenceRecordID: record.ID, ArtifactHash: actualHash, Endpoint: screening.Endpoint,
		RequestID: screening.RequestID, RequestHash: screening.RequestHash, ResponseHash: screening.ResponseHash,
		RetrievedAt: screening.RetrievedAt, AlgorithmVersion: screening.AlgorithmVersion,
		MinimumSpacingMeters: formatDecimal(screening.MinimumSpacingMeters, 0), MaxRequestSamples: screening.MaxRequestSamples,
		RequestedSampleCount: screening.RequestedSampleCount, ReturnedSampleCount: len(replayed.Results),
		BoundaryNearSampleCount: plan.BoundaryNearSamples, InteriorSampleCount: plan.InteriorSamples,
		MaximumSlopeDegrees: formatDecimal(maximum, 3), MaximumSlopeGradePercent: formatDecimal(SlopeGradePercent(maximum), 1),
		Status:     "SAMPLED_TERRAIN_SCREEN_PASSED",
		Limitation: strconv.Itoa(len(replayed.Results)) + " sampled locations returned slopes below the screening threshold. Unsampled terrain may contain different conditions; this does not establish whole-field slope suitability.",
	}, nil
}

func SlopeGradePercent(degrees float64) float64 {
	return math.Tan(degrees*math.Pi/180) * 100
}

func SurfaceSlopeThresholdDegrees() float64 {
	return math.Atan(0.06) * 180 / math.Pi
}

func SlopeExceedsSurfaceLimit(degrees float64) bool {
	return degrees > SurfaceSlopeThresholdDegrees()
}

func SlopeConversionForDegrees(degrees float64) SlopeConversion {
	return SlopeConversion{
		OriginalDegrees: formatDecimal(degrees, 3), DerivedGradePercent: formatDecimal(SlopeGradePercent(degrees), 1),
		ThresholdGradePercent: "6", ThresholdDegrees: formatDecimal(SurfaceSlopeThresholdDegrees(), 2),
		Formula:         "gradePercent = tan(degrees × π / 180) × 100; thresholdDegrees = atan(0.06) × 180 / π",
		PolicySourceURL: michiganBiosolidsURL,
	}
}

func formatDecimal(value float64, places int) string {
	return strconv.FormatFloat(value, 'f', places, 64)
}

func sameCoordinate(left, right float64) bool { return math.Abs(left-right) <= 1e-9 }

func configuredRole(configured []string, role string) bool {
	for _, candidate := range configured {
		if candidate == role {
			return true
		}
	}
	return false
}

func compactArtifactHash(raw []byte) (string, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	return artifactHash(compact.Bytes()), nil
}

func resolutionError(code string, err error) error { return &ResolutionError{Code: code, err: err} }

func artifactHash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func decodeStrict(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("artifact contains trailing JSON")
	}
	return nil
}

func validateBoundaryGeometry(geometry polygonGeometry) ([][]float64, string, error) {
	if geometry.Type != "Polygon" || len(geometry.Coordinates) != 1 || len(geometry.Coordinates[0]) < 4 {
		return nil, "", errors.New("a single-ring GeoJSON polygon is required")
	}
	ring := geometry.Coordinates[0]
	first, last := ring[0], ring[len(ring)-1]
	if len(first) != 2 || len(last) != 2 || first[0] != last[0] || first[1] != last[1] {
		return nil, "", errors.New("polygon ring must be closed")
	}
	for _, point := range ring {
		if len(point) != 2 || point[0] < -180 || point[0] > 180 || point[1] < -90 || point[1] > 90 {
			return nil, "", errors.New("polygon contains an invalid coordinate")
		}
	}
	if err := validateSimpleRing(ring); err != nil {
		return nil, "", err
	}
	area := polygonAreaAcres(ring)
	if area <= 0.1 {
		return nil, "", errors.New("polygon area is not usable")
	}
	area = math.Round(area*10) / 10
	return ring, strconv.FormatFloat(area, 'f', 1, 64), nil
}

func validateSimpleRing(ring [][]float64) error {
	for index := 0; index < len(ring)-1; index++ {
		if samePoint(ring[index], ring[index+1]) {
			return errors.New("polygon contains a zero-length edge")
		}
		for other := index + 1; other < len(ring)-1; other++ {
			if samePoint(ring[index], ring[other]) {
				return errors.New("polygon contains a repeated non-closing vertex")
			}
		}
	}
	edges := len(ring) - 1
	for left := 0; left < edges; left++ {
		for right := left + 1; right < edges; right++ {
			if right == left+1 || (left == 0 && right == edges-1) {
				continue
			}
			if segmentsIntersect(ring[left], ring[left+1], ring[right], ring[right+1]) {
				return errors.New("polygon ring self-intersects")
			}
		}
	}
	return nil
}

func polygonContainsRing(parent, child [][]float64) bool {
	for _, point := range child[:len(child)-1] {
		if !pointInPolygonOrBoundary(point[0], point[1], parent) {
			return false
		}
	}
	for childIndex := 0; childIndex < len(child)-1; childIndex++ {
		left, right := child[childIndex], child[childIndex+1]
		midpoint := []float64{(left[0] + right[0]) / 2, (left[1] + right[1]) / 2}
		if !pointInPolygonOrBoundary(midpoint[0], midpoint[1], parent) {
			return false
		}
		for parentIndex := 0; parentIndex < len(parent)-1; parentIndex++ {
			if segmentsProperlyIntersect(left, right, parent[parentIndex], parent[parentIndex+1]) {
				return false
			}
		}
	}
	return true
}

func samePoint(left, right []float64) bool {
	return sameCoordinate(left[0], right[0]) && sameCoordinate(left[1], right[1])
}

func orientation(a, b, c []float64) float64 {
	return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
}

func pointOnSegment(point, left, right []float64) bool {
	const epsilon = 1e-10
	if math.Abs(orientation(left, right, point)) > epsilon {
		return false
	}
	return point[0] >= math.Min(left[0], right[0])-epsilon && point[0] <= math.Max(left[0], right[0])+epsilon && point[1] >= math.Min(left[1], right[1])-epsilon && point[1] <= math.Max(left[1], right[1])+epsilon
}

func segmentsIntersect(a, b, c, d []float64) bool {
	o1, o2, o3, o4 := orientation(a, b, c), orientation(a, b, d), orientation(c, d, a), orientation(c, d, b)
	if ((o1 > 0 && o2 < 0) || (o1 < 0 && o2 > 0)) && ((o3 > 0 && o4 < 0) || (o3 < 0 && o4 > 0)) {
		return true
	}
	return (math.Abs(o1) <= 1e-10 && pointOnSegment(c, a, b)) || (math.Abs(o2) <= 1e-10 && pointOnSegment(d, a, b)) || (math.Abs(o3) <= 1e-10 && pointOnSegment(a, c, d)) || (math.Abs(o4) <= 1e-10 && pointOnSegment(b, c, d))
}

func segmentsProperlyIntersect(a, b, c, d []float64) bool {
	o1, o2, o3, o4 := orientation(a, b, c), orientation(a, b, d), orientation(c, d, a), orientation(c, d, b)
	return ((o1 > 1e-10 && o2 < -1e-10) || (o1 < -1e-10 && o2 > 1e-10)) && ((o3 > 1e-10 && o4 < -1e-10) || (o3 < -1e-10 && o4 > 1e-10))
}

func polygonAreaAcres(ring [][]float64) float64 {
	meanLatitude := 0.0
	for _, point := range ring[:len(ring)-1] {
		meanLatitude += point[1]
	}
	meanLatitude /= float64(len(ring) - 1)
	metersPerLongitudeDegree := 111320.0 * math.Cos(meanLatitude*math.Pi/180)
	const metersPerLatitudeDegree = 110574.0
	area := 0.0
	for index := 0; index < len(ring)-1; index++ {
		left, right := ring[index], ring[index+1]
		x1, y1 := left[0]*metersPerLongitudeDegree, left[1]*metersPerLatitudeDegree
		x2, y2 := right[0]*metersPerLongitudeDegree, right[1]*metersPerLatitudeDegree
		area += x1*y2 - x2*y1
	}
	return math.Abs(area) / 2 / 4046.8564224
}

type slopeSample struct {
	Index               int
	Latitude, Longitude float64
	SlopeDegrees        float64
}

func highSlopeSamples(source []byte) ([]slopeSample, error) {
	var payload struct {
		Results []struct {
			Index  int     `json:"index"`
			OK     bool    `json:"ok"`
			Lat    float64 `json:"lat"`
			Lng    float64 `json:"lng"`
			Fields map[string]struct {
				Value  json.RawMessage `json:"value"`
				Status string          `json:"status"`
			} `json:"fields"`
		} `json:"results"`
	}
	if err := json.Unmarshal(source, &payload); err != nil {
		return nil, err
	}
	result := []slopeSample{}
	for _, item := range payload.Results {
		field, ok := item.Fields["slope_degrees"]
		if !item.OK || !ok || field.Status != "ok" {
			continue
		}
		var slope float64
		if err := json.Unmarshal(field.Value, &slope); err != nil {
			return nil, err
		}
		if SlopeExceedsSurfaceLimit(slope) {
			result = append(result, slopeSample{Index: item.Index, Latitude: item.Lat, Longitude: item.Lng, SlopeDegrees: slope})
		}
	}
	return result, nil
}

func maximumSlopeDegrees(source []byte) (float64, error) {
	var payload struct {
		Results []struct {
			OK     bool `json:"ok"`
			Fields map[string]struct {
				Value  json.RawMessage `json:"value"`
				Status string          `json:"status"`
			} `json:"fields"`
		} `json:"results"`
	}
	if err := json.Unmarshal(source, &payload); err != nil {
		return 0, err
	}
	maximum := -1.0
	for _, item := range payload.Results {
		fact, ok := item.Fields["slope_degrees"]
		if !item.OK || !ok || fact.Status != "ok" {
			continue
		}
		var slope float64
		if err := json.Unmarshal(fact.Value, &slope); err != nil {
			return 0, err
		}
		maximum = math.Max(maximum, slope)
	}
	if maximum < 0 {
		return 0, errors.New("source evidence contains no usable slope sample")
	}
	return maximum, nil
}

func pointInPolygonOrBoundary(longitude, latitude float64, ring [][]float64) bool {
	point := []float64{longitude, latitude}
	for index := 0; index < len(ring)-1; index++ {
		if pointOnSegment(point, ring[index], ring[index+1]) {
			return true
		}
	}
	return pointInPolygon(longitude, latitude, ring)
}

func pointInPolygon(longitude, latitude float64, ring [][]float64) bool {
	inside := false
	for current, previous := 0, len(ring)-1; current < len(ring); previous, current = current, current+1 {
		xCurrent, yCurrent := ring[current][0], ring[current][1]
		xPrevious, yPrevious := ring[previous][0], ring[previous][1]
		intersects := (yCurrent > latitude) != (yPrevious > latitude) && longitude < (xPrevious-xCurrent)*(latitude-yCurrent)/(yPrevious-yCurrent)+xCurrent
		if intersects {
			inside = !inside
		}
	}
	return inside
}
