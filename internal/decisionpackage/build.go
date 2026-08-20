package decisionpackage

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/lab"
	"github.com/salarkhannn/pfas-load-control/internal/placement"
	"github.com/salarkhannn/pfas-load-control/internal/policy"
	"github.com/salarkhannn/pfas-load-control/internal/responseplan"
)

func buildSnapshot(decision policy.Decision, report lab.Report, plan *placement.PlacementPlan, response *responseplan.ResponseRun, physical map[string]evidence.Evaluation) Snapshot {
	draft := report.Draft
	labSnapshot := LabSnapshot{
		ReportID: report.ID, ReportVersion: draft.Version, OriginalFilename: report.OriginalFilename,
		MediaType: report.MediaType, SHA256: report.SHA256, Laboratory: draft.Laboratory,
		SampleIdentifier: draft.SampleIdentifier, CollectionDate: draft.CollectionDate, Matrix: draft.Matrix,
		Method: draft.Method, Basis: draft.Basis, Analytes: make([]LabAnalyte, 0, len(draft.Analytes)),
		Gaps: []PackageGap{}, ConfirmedAt: report.ConfirmedAt,
	}
	for _, analyte := range draft.Analytes {
		var upperBound *string
		if analyte.IsNonDetect {
			upperBound = analyte.NormalizedReportingLimitUGKGDry
			if upperBound == nil {
				upperBound = analyte.NormalizedDetectionLimitUGKGDry
			}
		}
		labSnapshot.Analytes = append(labSnapshot.Analytes, LabAnalyte{
			Name: analyte.CanonicalAnalyte, ResultText: analyte.ResultText,
			ValueUGKGDry: analyte.NormalizedValueUGKGDry, UpperBound: upperBound,
			IsNonDetect: analyte.IsNonDetect, SourcePage: analyte.SourcePage, SourceExcerpt: analyte.SourceExcerpt,
		})
	}
	for _, gap := range report.Gaps {
		if gap.Status == "RESOLVED" {
			continue
		}
		labSnapshot.Gaps = append(labSnapshot.Gaps, PackageGap{Source: "LAB", Code: gap.Code, Detail: gap.Detail, Resolution: gap.Resolution, Critical: true})
	}
	gaps := append([]PackageGap{}, labSnapshot.Gaps...)
	physicalEvidence := make([]evidence.Evaluation, 0, len(physical))
	physicalIDs := make([]string, 0, len(physical))
	for id := range physical {
		physicalIDs = append(physicalIDs, id)
	}
	sort.Strings(physicalIDs)
	for _, id := range physicalIDs {
		physicalEvidence = append(physicalEvidence, physical[id])
	}
	if plan != nil {
		for _, gap := range plan.Gaps {
			gaps = append(gaps, PackageGap{Source: "PLACEMENT", Code: gap.Code, Detail: gap.Detail, Resolution: gap.Resolution, Critical: true})
		}
		for _, field := range plan.Fields {
			if field.PhysicalEvaluationID == "" {
				continue
			}
			for _, gap := range physical[field.PhysicalEvaluationID].Gaps {
				gaps = append(gaps, PackageGap{Source: "PHYSICAL_EVIDENCE", Code: gap.Code, Detail: gap.Detail, Resolution: "Review the source record and resolve the missing condition.", Critical: gap.Critical})
			}
		}
	}
	if response != nil {
		for _, gap := range response.DataGaps {
			gaps = append(gaps, PackageGap{Source: "PFAS_RESPONSE", Code: gap.Code, Detail: gap.Detail, Resolution: gap.Resolution, Critical: gap.Critical})
		}
	}
	return Snapshot{Decision: decision, Lab: labSnapshot, Placement: plan, PhysicalEvidence: physicalEvidence, Response: response, Gaps: deduplicateGaps(gaps)}
}

func buildEvidenceLedger(snapshot Snapshot) []EvidenceEntry {
	entries := []EvidenceEntry{
		{
			Kind: "LAB_REPORT", Provider: "Laboratory report", Title: "Confirmed PFAS laboratory evidence",
			Status: "AVAILABLE", RecordID: snapshot.Lab.ReportID, SourceHash: snapshot.Lab.SHA256,
			Detail:      fmt.Sprintf("Version %d of %s was confirmed with %d analyte results.", snapshot.Lab.ReportVersion, snapshot.Lab.OriginalFilename, len(snapshot.Lab.Analytes)),
			RetrievedAt: snapshot.Lab.ConfirmedAt,
		},
		{
			Kind: "POLICY_RULE_PACK", Provider: "Michigan EGLE", Title: snapshot.Decision.RulePack.SourceTitle,
			Status: "AVAILABLE", RecordID: snapshot.Decision.ID, SourceURL: snapshot.Decision.RulePack.SourceURL,
			SourceVersion: snapshot.Decision.RulePack.Version, SourceHash: snapshot.Decision.RulePack.Checksum,
			Detail:      fmt.Sprintf("Rule %s produced the %s batch tier.", valueOr(snapshot.Decision.MatchedRuleID, "unmatched"), snapshot.Decision.Tier),
			RetrievedAt: timePointer(snapshot.Decision.RulePack.RetrievedAt),
		},
	}
	if snapshot.Placement != nil {
		entries = append(entries, EvidenceEntry{
			Kind: "PLACEMENT_PLAN", Provider: "FieldProof", Title: "Deterministic draft placement plan",
			Status: evidenceStatus(snapshot.Placement.Status == placement.StatusReady), RecordID: snapshot.Placement.ID,
			SourceVersion: snapshot.Placement.ConfigVersion, SourceHash: snapshot.Placement.InputHash,
			Detail:      fmt.Sprintf("%s dry tons allocated; %s dry tons remain.", snapshot.Placement.AllocatedDryTons, snapshot.Placement.UnallocatedDryTons),
			RetrievedAt: timePointer(snapshot.Placement.CreatedAt), Caveat: "Draft only. It is not approval to schedule or perform land application.",
		})
	}
	for _, item := range snapshot.PhysicalEvidence {
		data, _ := json.Marshal(struct {
			SampleCount      int `json:"sampleCount"`
			ProjectedCredits int `json:"projectedCredits"`
			FactCount        int `json:"factCount"`
		}{item.SampleCount, item.ProjectedCredits, len(item.Facts)})
		entries = append(entries, EvidenceEntry{
			Kind: "PHYSICAL_EVIDENCE", Provider: "Mireye and cited source datasets", Title: "Field physical evidence",
			Status: evidenceStatus(item.Status == evidence.StatusSucceeded && len(item.Gaps) == 0), RecordID: item.ID,
			SourceVersion: item.FieldSetVersion + "/" + item.AggregationVersion,
			Detail:        fmt.Sprintf("%d conditions aggregated across %d confirmed field points.", len(item.Facts), item.SampleCount),
			RetrievedAt:   item.CompletedAt, Caveat: "Unavailable values and source caveats remain explicit in the evidence record.", Data: data,
		})
	}
	if snapshot.Response != nil {
		entries = append(entries, EvidenceEntry{
			Kind: "PFAS_RESPONSE", Provider: "FieldProof", Title: "Required PFAS response dossier",
			Status: evidenceStatus(snapshot.Response.Status == "READY"), RecordID: snapshot.Response.ID,
			SourceURL: snapshot.Response.PolicySourceURL, SourceVersion: snapshot.Response.PolicyVersion,
			Detail:      fmt.Sprintf("%d required actions, %d investigation leads, and %d alternative-management candidates were assembled.", len(snapshot.Response.Tasks), len(snapshot.Response.InvestigationLeads), len(snapshot.Response.Alternatives)),
			RetrievedAt: timePointer(snapshot.Response.UpdatedAt), Caveat: "Geographic leads do not prove sewer connection, PFAS use, release, or causation.",
		})
		for _, item := range snapshot.Response.Evidence {
			entries = append(entries, EvidenceEntry{
				Kind: item.Kind, Provider: item.Provider, Title: item.Title, Status: item.Status,
				SourceURL: item.SourceURL, SourceVersion: item.SourceVintage, RetrievedAt: timePointer(item.FetchedAt),
				Detail: item.Summary, Caveat: item.Caveat, Data: append(json.RawMessage(nil), item.Data...),
			})
		}
	}
	for index := range entries {
		entries[index].Position = index + 1
	}
	return entries
}

func buildProposedActions(snapshot Snapshot) []ProposedAction {
	actions := make([]ProposedAction, 0, len(snapshot.Decision.Requirements)+8)
	seen := make(map[string]bool)
	add := func(action ProposedAction) {
		if seen[action.Code] {
			return
		}
		seen[action.Code] = true
		action.Position = len(actions) + 1
		action.Executable = false
		actions = append(actions, action)
	}
	responseTasks := make(map[string]responseplan.ResponseTask)
	if snapshot.Response != nil {
		for _, task := range snapshot.Response.Tasks {
			responseTasks[task.Code] = task
		}
	}
	consumedResponseTasks := make(map[string]bool)
	for _, requirement := range snapshot.Decision.Requirements {
		code := "POLICY_" + requirement.ID
		state := "REQUIRED"
		if responseCode := responseCodeForRequirement(requirement.ID); responseCode != "" {
			if task, ok := responseTasks[responseCode]; ok {
				code = task.Code
				state = task.State
				consumedResponseTasks[task.Code] = true
			}
		}
		add(ProposedAction{Code: code, Category: "REGULATORY", State: state, Title: requirement.Title, Detail: requirement.Detail, Timing: requirement.Timing, SourceID: requirement.RuleID})
	}
	if snapshot.Response != nil {
		for _, task := range snapshot.Response.Tasks {
			if consumedResponseTasks[task.Code] {
				continue
			}
			add(ProposedAction{Code: task.Code, Category: task.Category, State: task.State, Title: task.Title, Detail: task.Detail, Timing: task.Timing, SourceID: snapshot.Response.ID})
		}
	}
	if snapshot.Placement != nil {
		if snapshot.Placement.Status == placement.StatusReady {
			add(ProposedAction{Code: "REVIEW_DRAFT_ALLOCATION", Category: "PLACEMENT", State: "DRAFT", Title: "Review the proposed field allocation", Detail: fmt.Sprintf("Review the allocation of %s dry tons across %d eligible field(s) against the approved Residuals Management Program before scheduling.", snapshot.Placement.AllocatedDryTons, len(snapshot.Placement.Allocations)), Timing: "Before scheduling", SourceID: snapshot.Placement.ID})
		} else {
			add(ProposedAction{Code: "RESOLVE_PLACEMENT_GAPS", Category: "EVIDENCE", State: "REQUIRED", Title: "Resolve the placement evidence gaps", Detail: "No operational allocation should be used until every blocking field and capacity gap in this package is resolved.", Timing: "Before scheduling", SourceID: snapshot.Placement.ID})
		}
	}
	if snapshot.Decision.Tier == policy.TierProhibited {
		add(ProposedAction{Code: "BLOCK_LAND_APPLICATION", Category: "CONTROL", State: "ENFORCED", Title: "Keep this batch out of land application", Detail: "The deterministic control plane blocks land-application allocation for this batch.", Timing: "Effective immediately", SourceID: snapshot.Decision.ID})
	}
	return actions
}

func responseCodeForRequirement(requirementID string) string {
	return map[string]string{
		"MI-PFAS-WRD-NOTIFICATION":            "NOTIFY_EGLE",
		"MI-PFAS-ELEVATED-RATE":               "ENFORCE_RATE_CAP",
		"MI-PFAS-SOURCE-EFFLUENT-30D":         "SOURCE_EFFLUENT_SAMPLE",
		"MI-PFAS-LAND-APPLICATION-PROHIBITED": "BLOCK_LAND_APPLICATION",
		"MI-PFAS-ALTERNATIVE-MANAGEMENT":      "ARRANGE_ALTERNATIVE_MANAGEMENT",
	}[requirementID]
}

func packageStatus(snapshot Snapshot) string {
	if len(snapshot.Gaps) > 0 {
		return "REVIEW_REQUIRED"
	}
	if snapshot.Placement != nil && snapshot.Placement.Status != placement.StatusReady {
		return "REVIEW_REQUIRED"
	}
	if snapshot.Response != nil && snapshot.Response.Status != "READY" {
		return "REVIEW_REQUIRED"
	}
	return "READY"
}

func hydrate(record db.PfasDecisionPackage) (DecisionPackage, error) {
	result := DecisionPackage{
		ID: record.ID.String(), DecisionID: record.DecisionID.String(), SchemaVersion: record.SchemaVersion,
		Status: record.Status, InputHash: record.InputHash, CreatedAt: record.CreatedAt.Time,
	}
	if err := json.Unmarshal(record.Snapshot, &result.Snapshot); err != nil {
		return DecisionPackage{}, fmt.Errorf("decode decision package snapshot: %w", err)
	}
	if err := json.Unmarshal(record.EvidenceLedger, &result.Evidence); err != nil {
		return DecisionPackage{}, fmt.Errorf("decode decision package evidence: %w", err)
	}
	if err := json.Unmarshal(record.ProposedActions, &result.ProposedActions); err != nil {
		return DecisionPackage{}, fmt.Errorf("decode decision package actions: %w", err)
	}
	result.Artifacts = []Artifact{
		{Format: "html", MediaType: "text/html", SHA256: record.HtmlSha256, SizeBytes: len([]byte(record.HtmlArtifact)), URL: artifactURL(result.ID, "html")},
		{Format: "pdf", MediaType: "application/pdf", SHA256: record.PdfSha256, SizeBytes: len(record.PdfArtifact), URL: artifactURL(result.ID, "pdf")},
		{Format: "json", MediaType: "application/json", SHA256: record.JsonSha256, SizeBytes: len(record.JsonArtifact), URL: artifactURL(result.ID, "json")},
	}
	return result, nil
}

func artifactMetadata(id string, jsonBytes, htmlBytes, pdfBytes []byte) []Artifact {
	return []Artifact{
		{Format: "html", MediaType: "text/html", SHA256: digest(htmlBytes), SizeBytes: len(htmlBytes), URL: artifactURL(id, "html")},
		{Format: "pdf", MediaType: "application/pdf", SHA256: digest(pdfBytes), SizeBytes: len(pdfBytes), URL: artifactURL(id, "pdf")},
		{Format: "json", MediaType: "application/json", SHA256: digest(jsonBytes), SizeBytes: len(jsonBytes), URL: artifactURL(id, "json")},
	}
}

func artifactURL(id, format string) string {
	return "/api/v1/decision-packages/" + id + "/exports/" + format
}

func deduplicateGaps(gaps []PackageGap) []PackageGap {
	result := make([]PackageGap, 0, len(gaps))
	seen := make(map[string]bool)
	for _, gap := range gaps {
		key := gap.Source + ":" + gap.Code
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, gap)
	}
	return result
}

func evidenceStatus(ready bool) string {
	if ready {
		return "AVAILABLE"
	}
	return "PARTIAL"
}

func valueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func timePointer(value time.Time) *time.Time { return &value }
