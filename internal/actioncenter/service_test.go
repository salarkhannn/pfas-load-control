package actioncenter

import (
	"strings"
	"testing"

	"github.com/salarkhannn/pfas-load-control/internal/decisionpackage"
)

func TestHashPayloadBindsEveryEditableValue(t *testing.T) {
	base := ActionPayload{
		Channel: "MIENVIRO", Recipient: "Michigan EGLE", Subject: "Submit results", Message: "Submit the frozen result.",
		Attachments: []ActionAttachment{{Label: "Package", Format: "pdf", MediaType: "application/pdf", SHA256: strings.Repeat("a", 64), URL: "/package.pdf"}},
	}
	first := hashPayload(base)
	if len(first) != 64 {
		t.Fatalf("hash length = %d, want 64", len(first))
	}
	changed := base
	changed.Message = "Submit a changed result."
	if hashPayload(changed) == first {
		t.Fatal("message change did not change the payload hash")
	}
}

func TestActionModeKeepsControlsOutOfApproval(t *testing.T) {
	mode, approval := actionMode(decisionpackage.ProposedAction{Code: "ENFORCE_RATE_CAP"})
	if mode != ModeControl || approval {
		t.Fatalf("control mode = %s, approval = %t", mode, approval)
	}
	mode, approval = actionMode(decisionpackage.ProposedAction{Code: "REVIEW_DRAFT_ALLOCATION"})
	if mode != ModeInternalRelease || !approval {
		t.Fatalf("release mode = %s, approval = %t", mode, approval)
	}
	mode, approval = actionMode(decisionpackage.ProposedAction{Code: "SOURCE_EFFLUENT_SAMPLE"})
	if mode != ModeOperatorHandoff || !approval {
		t.Fatalf("handoff mode = %s, approval = %t", mode, approval)
	}
}

func TestSplitGapsPreservesApprovalBoundary(t *testing.T) {
	critical, review := splitGaps([]decisionpackage.PackageGap{
		{Code: "MISSING_METHOD", Detail: "Method missing", Resolution: "Confirm method", Critical: true},
		{Code: "PARTIAL_ROAD", Detail: "Road surface missing", Resolution: "Review route"},
	})
	if len(critical) != 1 || critical[0].Code != "MISSING_METHOD" {
		t.Fatalf("critical gaps = %#v", critical)
	}
	if len(review) != 1 || review[0].Code != "PARTIAL_ROAD" {
		t.Fatalf("review gaps = %#v", review)
	}
}

func TestDefaultPayloadNeverClaimsExternalDelivery(t *testing.T) {
	pkg := decisionpackage.DecisionPackage{Artifacts: []decisionpackage.Artifact{{Format: "pdf", MediaType: "application/pdf", SHA256: strings.Repeat("b", 64), URL: "/package.pdf"}}}
	payload := defaultPayload(pkg, decisionpackage.ProposedAction{Code: "SOURCE_EFFLUENT_SAMPLE", Title: "Collect a sample", Detail: "Collect a representative sample", Timing: "Within 30 days"}, ModeOperatorHandoff)
	if payload.Channel != "SAMPLING_REQUEST" || payload.Recipient != "" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Attachments) != 1 || payload.Attachments[0].SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("attachments = %#v", payload.Attachments)
	}
}
