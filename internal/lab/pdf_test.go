package lab

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestParseTesseractTSV(t *testing.T) {
	t.Parallel()
	input := []byte("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t10\t10\t40\t10\t95\tPFOS\n" +
		"5\t1\t1\t1\t1\t2\t55\t10\t40\t10\t94\t12.5\n" +
		"5\t1\t1\t1\t2\t1\t10\t30\t40\t10\t96\tPFOA\n")
	actual, err := parseTesseractTSV(input)
	if err != nil {
		t.Fatal(err)
	}
	if actual != "PFOS 12.5\nPFOA" {
		t.Fatalf("OCR text = %q", actual)
	}
}

func TestSystemPDFExtractorReadsMachineTextWithPages(t *testing.T) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo is not installed")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not installed")
	}
	lines := []string{
		"Laboratory: Great Lakes Lab",
		"Sample ID: BATCH-42",
		"Collection Date: 2026-08-01",
		"Matrix: Biosolids",
		"Method: EPA 1633",
		"Basis: dry weight",
		"PFOS 19.99 ug/kg dry DL 0.5",
		"PFOA ND ug/kg dry RL 1.0 DL 0.5",
	}
	pages, err := NewSystemPDFExtractor().Extract(context.Background(), textPDF(lines))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].ExtractionMethod != "PDF_TEXT" {
		t.Fatalf("unexpected pages: %#v", pages)
	}
	draft, gaps := validateDraft(parsePDFPages(pages), pages, nil)
	if len(gaps) != 0 {
		t.Fatalf("unexpected gaps: %#v\ntext:\n%s", gaps, pages[0].Text)
	}
	if len(draft.Analytes) != 2 || draft.Analytes[1].ReportingLimit == nil {
		t.Fatalf("unexpected analytes: %#v", draft.Analytes)
	}
}

func textPDF(lines []string) []byte {
	var content strings.Builder
	content.WriteString("BT\n/F1 10 Tf\n50 750 Td\n")
	for index, line := range lines {
		if index > 0 {
			content.WriteString("0 -18 Td\n")
		}
		content.WriteString("(")
		content.WriteString(strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(line))
		content.WriteString(") Tj\n")
	}
	content.WriteString("ET\n")
	stream := content.String()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", len(objects)+1)
	output.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		output.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}
