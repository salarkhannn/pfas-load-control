package decisionpackage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/go-pdf/fpdf"
)

type packageDocument struct {
	ID              string           `json:"id"`
	DecisionID      string           `json:"decisionId"`
	SchemaVersion   string           `json:"schemaVersion"`
	Status          string           `json:"status"`
	InputHash       string           `json:"inputHash"`
	Snapshot        Snapshot         `json:"snapshot"`
	Evidence        []EvidenceEntry  `json:"evidence"`
	ProposedActions []ProposedAction `json:"proposedActions"`
	CreatedAt       string           `json:"createdAt"`
}

func exportDocument(value DecisionPackage) packageDocument {
	return packageDocument{
		ID: value.ID, DecisionID: value.DecisionID, SchemaVersion: value.SchemaVersion,
		Status: value.Status, InputHash: value.InputHash, Snapshot: value.Snapshot,
		Evidence: value.Evidence, ProposedActions: value.ProposedActions,
		CreatedAt: value.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

var documentTemplate = template.Must(template.New("package").Funcs(template.FuncMap{
	"date":   func(value interface{ Format(string) string }) string { return value.Format("Jan 2, 2006 15:04 UTC") },
	"status": func(value string) string { return strings.ReplaceAll(strings.ToLower(value), "_", " ") },
	"value":  printableJSON,
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>PFAS decision package · {{.Snapshot.Decision.BatchIdentifier}}</title>
<style>
:root{font-family:Arial,sans-serif;color:#1f2929;background:#f7f8f6;line-height:1.45}*{box-sizing:border-box}body{margin:0}.page{max-width:980px;margin:0 auto;padding:48px 32px 72px}.eyebrow{color:#127783;font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase}h1{font-size:34px;margin:8px 0 4px}h2{font-size:21px;margin:0}h3{font-size:16px;margin:0 0 4px}p{margin:4px 0;color:#566161}.cover,.section{background:#fff;border:1px solid #dfe3df;border-radius:12px;margin-bottom:20px}.cover-head{display:grid;grid-template-columns:1fr 240px;gap:24px;padding:28px}.status{padding:18px;border-left:1px solid #dfe3df}.status strong{display:block;font-size:18px;text-transform:capitalize}.meta{display:grid;grid-template-columns:repeat(3,1fr);border-top:1px solid #dfe3df}.meta div{padding:16px 20px;border-right:1px solid #dfe3df}.meta div:last-child{border-right:0}dt{font-size:11px;color:#6e7977;text-transform:uppercase;letter-spacing:.05em}dd{margin:3px 0 0;font-weight:600}.section>header{padding:20px 24px;border-bottom:1px solid #e5e8e5}.row{display:grid;grid-template-columns:42px 1fr 150px;gap:16px;padding:18px 24px;border-bottom:1px solid #eceeec}.row:last-child{border-bottom:0}.number{color:#85908e;font-variant-numeric:tabular-nums}.tag{font-size:12px;font-weight:700;text-transform:uppercase;color:#127783}.review{color:#a86410}.source{font-size:12px;word-break:break-word}.notice{padding:16px 24px;background:#fff8ec;border-bottom:1px solid #eed7af;color:#7b551e}.hash{font-family:monospace;font-size:11px;word-break:break-all}.foot{font-size:12px;color:#66716f;padding-top:12px}@media(max-width:680px){.page{padding:24px 14px}.cover-head{grid-template-columns:1fr}.status{border-left:0;border-top:1px solid #dfe3df;padding-left:0}.meta{grid-template-columns:1fr}.meta div{border-right:0;border-bottom:1px solid #dfe3df}.row{grid-template-columns:30px 1fr}.row>div:last-child{grid-column:2}}
@media print{body{background:#fff}.page{max-width:none;padding:0}.cover,.section{break-inside:avoid;border-color:#bbb}.row{break-inside:avoid}}
</style></head><body><main class="page">
<section class="cover"><div class="cover-head"><div><span class="eyebrow">PFAS decision package</span><h1>{{.Snapshot.Decision.BatchIdentifier}}</h1><p>{{.Snapshot.Decision.FacilityName}}</p><p>{{.Snapshot.Decision.Explanation}}</p></div><div class="status"><span class="eyebrow">Package status</span><strong class="{{if eq .Status "REVIEW_REQUIRED"}}review{{end}}">{{status .Status}}</strong><p>Generated {{date .CreatedAt}}</p></div></div><dl class="meta"><div><dt>PFAS tier</dt><dd>{{.Snapshot.Decision.Tier}}</dd></div><div><dt>Lab report</dt><dd>{{.Snapshot.Lab.OriginalFilename}}</dd></div><div><dt>Package version</dt><dd>{{.SchemaVersion}}</dd></div></dl></section>
{{if .Snapshot.Gaps}}<section class="section"><header><span class="eyebrow">Open evidence</span><h2>Items requiring review</h2></header>{{range .Snapshot.Gaps}}<div class="notice"><strong>{{.Source}} · {{.Code}}</strong><p>{{.Detail}}</p><p><b>Resolve:</b> {{.Resolution}}</p></div>{{end}}</section>{{end}}
<section class="section"><header><span class="eyebrow">Proposed actions</span><h2>What happens next</h2></header>{{range .ProposedActions}}<article class="row"><span class="number">{{printf "%02d" .Position}}</span><div><h3>{{.Title}}</h3><p>{{.Detail}}</p></div><div><span class="tag">{{.State}}</span><p>{{.Timing}}</p></div></article>{{end}}</section>
<section class="section"><header><span class="eyebrow">Evidence ledger</span><h2>Sources frozen in this package</h2></header>{{range .Evidence}}<article class="row"><span class="number">{{printf "%02d" .Position}}</span><div><h3>{{.Title}}</h3><p>{{.Detail}}</p>{{if .Caveat}}<p><b>Caveat:</b> {{.Caveat}}</p>{{end}}{{if .SourceHash}}<p class="hash">{{.SourceHash}}</p>{{end}}</div><div><span class="tag {{if ne .Status "AVAILABLE"}}review{{end}}">{{.Status}}</span><p class="source">{{.Provider}}</p>{{if .SourceURL}}<a class="source" href="{{.SourceURL}}">Open source</a>{{end}}</div></article>{{end}}</section>
{{range .Snapshot.PhysicalEvidence}}<section class="section"><header><span class="eyebrow">Physical evidence</span><h2>Confirmed field evaluation</h2><p>Evaluation {{.ID}} · geometry version {{.GeometryVersion}} · {{.SampleCount}} field points</p></header>{{range .Facts}}<article class="row"><span class="number">{{.Category}}</span><div><h3>{{.Label}}</h3><p>{{value .Value}}{{if .Unit}} {{.Unit}}{{end}}</p><p>{{.AggregateMethod}} across {{.OKCount}} available samples</p></div><div><span class="tag {{if ne .State "COMPLETE"}}review{{end}}">{{.State}}</span><p class="source">{{.Source}}</p>{{if .SourceURL}}<a class="source" href="{{.SourceURL}}">Open source</a>{{end}}</div></article>{{end}}{{range .Supplemental}}<article class="row"><span class="number">RELATED</span><div><h3>{{.Title}}</h3><p>{{.Summary}}</p>{{if .Caveat}}<p><b>Caveat:</b> {{.Caveat}}</p>{{end}}</div><div><span class="tag {{if ne .Status "AVAILABLE"}}review{{end}}">{{.Status}}</span><p class="source">{{.Provider}}</p>{{if .SourceURL}}<a class="source" href="{{.SourceURL}}">Open source</a>{{end}}</div></article>{{end}}</section>{{end}}
<p class="foot">This package preserves evidence and proposed actions for human review. It does not approve, submit, schedule, notify, contact, or execute any action. Package ID {{.ID}} · Input hash <span class="hash">{{.InputHash}}</span></p>
</main></body></html>`))

func renderHTML(value DecisionPackage) (string, error) {
	var output bytes.Buffer
	if err := documentTemplate.Execute(&output, value); err != nil {
		return "", fmt.Errorf("render HTML decision package: %w", err)
	}
	return output.String(), nil
}

func renderPDF(value DecisionPackage) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	pdf.SetTitle(pdfText("PFAS decision package - "+value.Snapshot.Decision.BatchIdentifier), false)
	pdf.SetAuthor("PFAS Load Control", false)
	pdf.AddPage()

	pdf.SetTextColor(18, 119, 131)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(0, 6, "PFAS DECISION PACKAGE", "", 1, "L", false, 0, "")
	pdf.SetTextColor(31, 41, 41)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.MultiCell(0, 10, pdfText(value.Snapshot.Decision.BatchIdentifier), "", "L", false)
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(85, 97, 97)
	pdf.MultiCell(0, 6, pdfText(value.Snapshot.Decision.FacilityName+" | "+value.Status+" | "+string(value.Snapshot.Decision.Tier)), "", "L", false)
	pdf.Ln(4)
	pdf.SetTextColor(31, 41, 41)
	pdf.SetFont("Helvetica", "", 10)
	pdf.MultiCell(0, 5, pdfText(value.Snapshot.Decision.Explanation), "", "L", false)

	if len(value.Snapshot.Gaps) > 0 {
		pdfSection(pdf, "ITEMS REQUIRING REVIEW")
		for _, gap := range value.Snapshot.Gaps {
			pdfItem(pdf, gap.Source+" | "+gap.Code, gap.Detail+" Resolve: "+gap.Resolution, "REVIEW")
		}
	}

	pdfSection(pdf, "PROPOSED ACTIONS")
	for _, action := range value.ProposedActions {
		pdfItem(pdf, fmt.Sprintf("%02d  %s", action.Position, action.Title), action.Detail, action.State+" | "+action.Timing)
	}

	pdfSection(pdf, "EVIDENCE LEDGER")
	for _, item := range value.Evidence {
		detail := item.Detail
		if item.Caveat != "" {
			detail += " Caveat: " + item.Caveat
		}
		pdfItem(pdf, fmt.Sprintf("%02d  %s", item.Position, item.Title), detail, item.Status+" | "+item.Provider)
	}

	for _, evaluation := range value.Snapshot.PhysicalEvidence {
		pdfSection(pdf, "PHYSICAL EVIDENCE | FIELD EVALUATION")
		pdfItem(pdf, "Evaluation "+evaluation.ID, fmt.Sprintf("Geometry version %d; %d confirmed field points.", evaluation.GeometryVersion, evaluation.SampleCount), string(evaluation.Status))
		for _, fact := range evaluation.Facts {
			factValue := printableJSON(fact.Value)
			if fact.Unit != "" {
				factValue += " " + fact.Unit
			}
			pdfItem(pdf, fact.Label, factValue+". "+fact.AggregateMethod+fmt.Sprintf(" across %d available samples.", fact.OKCount), fact.State+" | "+fact.Source)
		}
		for _, item := range evaluation.Supplemental {
			pdfItem(pdf, item.Title, item.Summary+optionalSentence("Caveat: ", item.Caveat), item.Status+" | "+item.Provider)
		}
	}

	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(85, 97, 97)
	pdf.MultiCell(0, 4, pdfText("This package preserves evidence and proposed actions for human review. It does not approve, submit, schedule, notify, contact, or execute any action."), "", "L", false)
	pdf.MultiCell(0, 4, pdfText("Package ID: "+value.ID+" | Input hash: "+value.InputHash), "", "L", false)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, fmt.Errorf("render PDF decision package: %w", err)
	}
	return output.Bytes(), nil
}

func pdfSection(pdf *fpdf.Fpdf, title string) {
	pdf.Ln(8)
	pdf.SetTextColor(18, 119, 131)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(0, 6, title, "B", 1, "L", false, 0, "")
	pdf.SetTextColor(31, 41, 41)
}

func pdfItem(pdf *fpdf.Fpdf, title, detail, meta string) {
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.MultiCell(0, 5, pdfText(title), "", "L", false)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(85, 97, 97)
	pdf.MultiCell(0, 4.5, pdfText(detail), "", "L", false)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(18, 119, 131)
	pdf.MultiCell(0, 4, pdfText(meta), "", "L", false)
	pdf.SetTextColor(31, 41, 41)
}

func pdfText(value string) string {
	replacer := strings.NewReplacer("µ", "u", "—", "-", "–", "-", "’", "'", "‘", "'", "“", "\"", "”", "\"", "•", "-")
	return replacer.Replace(value)
}

func printableJSON(value json.RawMessage) string {
	if len(value) == 0 {
		return "Unavailable"
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return string(value)
	}
	if text, ok := decoded.(string); ok {
		return text
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return string(value)
	}
	return string(encoded)
}

func optionalSentence(prefix, value string) string {
	if value == "" {
		return ""
	}
	return " " + prefix + value
}
