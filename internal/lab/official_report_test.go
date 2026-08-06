package lab

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestParseMichiganMiEnviroBlissfieldBiosolidsReport(t *testing.T) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo is not installed")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not installed")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract is not installed")
	}
	content, err := os.ReadFile("../../sample-data/michigan-mienviro-blissfield-wwtp-biosolids-2025.pdf")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("local Michigan report fixture is not available")
	}
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewParser(NewSystemPDFExtractor()).Parse(context.Background(), "application/pdf", content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Gaps) != 0 {
		t.Fatalf("unexpected gaps: %#v\ndraft: %#v", result.Gaps, result.Draft)
	}
	if valueOrEmpty(result.Draft.Laboratory) != "Merit Laboratories, Inc." {
		t.Fatalf("laboratory = %q", valueOrEmpty(result.Draft.Laboratory))
	}
	if valueOrEmpty(result.Draft.SampleIdentifier) != "S70707.01" {
		t.Fatalf("sample identifier = %q", valueOrEmpty(result.Draft.SampleIdentifier))
	}
	if valueOrEmpty(result.Draft.CollectionDate) != "2025-01-22" {
		t.Fatalf("collection date = %q", valueOrEmpty(result.Draft.CollectionDate))
	}
	if valueOrEmpty(result.Draft.Matrix) != "BIOSOLIDS" || valueOrEmpty(result.Draft.Basis) != "DRY" {
		t.Fatalf("matrix/basis = %q/%q", valueOrEmpty(result.Draft.Matrix), valueOrEmpty(result.Draft.Basis))
	}
	if valueOrEmpty(result.Draft.Method) != "ASTM D7968-17M — ASTM Method D7968 - 17 Modified (Isotopic Dilution)" {
		t.Fatalf("method = %q", valueOrEmpty(result.Draft.Method))
	}
	if len(result.Draft.Analytes) != 2 {
		t.Fatalf("analytes = %#v", result.Draft.Analytes)
	}
	pfos, pfoa := result.Draft.Analytes[0], result.Draft.Analytes[1]
	if valueOrEmpty(pfos.Value) != "5.5" || valueOrEmpty(pfos.Unit) != "UG_KG" || valueOrEmpty(pfos.ReportingLimit) != "1.1" {
		t.Fatalf("PFOS = %#v", pfos)
	}
	if valueOrEmpty(pfoa.Value) != "3" || valueOrEmpty(pfoa.Unit) != "UG_KG" || valueOrEmpty(pfoa.ReportingLimit) != "1.1" {
		t.Fatalf("PFOA = %#v", pfoa)
	}
}
