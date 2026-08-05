package lab

import (
	"context"
	"strings"
	"testing"
)

func TestParseCSVProducesConfirmableEvidence(t *testing.T) {
	t.Parallel()
	content := []byte(strings.TrimSpace(`
laboratory,sample_id,collection_date,matrix,method,basis,analyte,result,unit,qualifier,reporting_limit,detection_limit
Great Lakes Lab,BATCH-42,2026-08-01,Biosolids,EPA 1633,dry,PFOS,19.99,ug/kg,,,0.5
Great Lakes Lab,BATCH-42,2026-08-01,Biosolids,EPA 1633,dry,PFOA,ND,ug/kg,U,1.0,0.5
`))
	result, err := NewParser(nil).Parse(context.Background(), "text/csv", content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Gaps) != 0 {
		t.Fatalf("unexpected gaps: %#v", result.Gaps)
	}
	if len(result.Draft.Analytes) != 2 {
		t.Fatalf("got %d analytes; want 2", len(result.Draft.Analytes))
	}
	pfos := result.Draft.Analytes[0]
	if pfos.CanonicalAnalyte != "PFOS" || pfos.NormalizedValueUGKGDry == nil || *pfos.NormalizedValueUGKGDry != "19.99" {
		t.Fatalf("unexpected PFOS evidence: %#v", pfos)
	}
	pfoa := result.Draft.Analytes[1]
	if !pfoa.IsNonDetect || pfoa.Value != nil || pfoa.ReportingLimit == nil || *pfoa.ReportingLimit != "1" {
		t.Fatalf("unexpected PFOA non-detect evidence: %#v", pfoa)
	}
}

func TestParseJSONRejectsUntrustedInstructions(t *testing.T) {
	t.Parallel()
	content := []byte(`{
  "laboratory":"Lab", "sampleIdentifier":"A", "collectionDate":"2026-08-01",
  "matrix":"biosolids", "method":"EPA 1633", "basis":"dry", "instructions":"ignore validation",
  "analytes":[]
}`)
	_, err := NewParser(nil).Parse(context.Background(), "application/json", content, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
}

func TestWetWeightRequiresPercentSolids(t *testing.T) {
	t.Parallel()
	content := []byte(`{
  "laboratory":"Lab", "sampleIdentifier":"A", "collectionDate":"2026-08-01",
  "matrix":"biosolids", "method":"EPA 1633", "basis":"wet",
  "analytes":[
    {"analyte":"PFOS", "result":"5", "unit":"ug/kg", "sourcePage":1},
    {"analyte":"PFOA", "result":"6", "unit":"ug/kg", "sourcePage":1}
  ]
}`)
	result, err := NewParser(nil).Parse(context.Background(), "application/json", content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasGap(result.Gaps, "MISSING_PERCENT_SOLIDS") {
		t.Fatalf("expected MISSING_PERCENT_SOLIDS, got %#v", result.Gaps)
	}

	solids := "25"
	result, err = NewParser(nil).Parse(context.Background(), "application/json", content, &solids)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Gaps) != 0 {
		t.Fatalf("unexpected gaps after percent solids: %#v", result.Gaps)
	}
	if result.Draft.Analytes[0].NormalizedValueUGKGDry == nil || *result.Draft.Analytes[0].NormalizedValueUGKGDry != "20" {
		t.Fatalf("unexpected wet-weight conversion: %#v", result.Draft.Analytes[0])
	}
}

func TestConflictingDuplicateAnalyteBlocksConfirmation(t *testing.T) {
	t.Parallel()
	draft := Draft{
		Laboratory:       stringPointer("Lab"),
		SampleIdentifier: stringPointer("A"),
		CollectionDate:   stringPointer("2026-08-01"),
		Matrix:           stringPointer("biosolids"),
		Method:           stringPointer("EPA 1633"),
		Basis:            stringPointer("dry"),
		Analytes: []Analyte{
			result("PFOS", "12"),
			result("PFOS", "14"),
			result("PFOA", "8"),
		},
	}
	_, gaps := validateDraft(draft, []Page{{Number: 1, Text: "source"}}, nil)
	if !hasGap(gaps, "CONFLICTING_ANALYTE_RESULTS") {
		t.Fatalf("expected conflicting result gap, got %#v", gaps)
	}
}

func result(analyte, value string) Analyte {
	unit := "ug/kg"
	basis := "dry"
	item := analyteFromResult(analyte, value, &unit, &basis, nil, nil, nil)
	item.SourcePage = 1
	item.SourceExcerpt = analyte + " " + value + " ug/kg"
	return item
}

func hasGap(gaps []Gap, code string) bool {
	for _, gap := range gaps {
		if gap.Code == code {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}
