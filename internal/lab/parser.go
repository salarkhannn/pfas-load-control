package lab

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

type PDFExtractor interface {
	Extract(context.Context, []byte) ([]Page, error)
}

type Parser struct {
	pdf PDFExtractor
}

func NewParser(pdf PDFExtractor) *Parser {
	return &Parser{pdf: pdf}
}

func (p *Parser) Parse(ctx context.Context, mediaType string, content []byte, percentSolids *string) (Extraction, error) {
	var (
		pages []Page
		draft Draft
		err   error
	)
	switch mediaType {
	case "text/csv":
		pages = []Page{{Number: 1, Text: string(content), ExtractionMethod: "CSV"}}
		draft, err = parseCSV(content)
	case "application/json":
		pages = []Page{{Number: 1, Text: string(content), ExtractionMethod: "JSON"}}
		draft, err = parseJSON(content)
	case "application/pdf":
		if p.pdf == nil {
			return Extraction{}, errors.New("PDF extraction is not configured")
		}
		pages, err = p.pdf.Extract(ctx, content)
		if err == nil {
			draft = parsePDFPages(pages)
		}
	default:
		return Extraction{}, fmt.Errorf("unsupported report media type %q", mediaType)
	}
	if err != nil {
		return Extraction{}, err
	}

	draft.Version = 1
	draft.Status = "DRAFT"
	draft.Source = "EXTRACTION"
	draft, gaps := validateDraft(draft, pages, percentSolids)
	return Extraction{Pages: pages, Draft: draft, Gaps: gaps}, nil
}

type jsonReport struct {
	Laboratory       *string       `json:"laboratory"`
	SampleIdentifier *string       `json:"sampleIdentifier"`
	CollectionDate   *string       `json:"collectionDate"`
	Matrix           *string       `json:"matrix"`
	Method           *string       `json:"method"`
	Basis            *string       `json:"basis"`
	Analytes         []jsonAnalyte `json:"analytes"`
}

type jsonAnalyte struct {
	Analyte        string  `json:"analyte"`
	Result         string  `json:"result"`
	Unit           *string `json:"unit"`
	Basis          *string `json:"basis"`
	Qualifier      *string `json:"qualifier"`
	ReportingLimit *string `json:"reportingLimit"`
	DetectionLimit *string `json:"detectionLimit"`
	SourcePage     int     `json:"sourcePage"`
	SourceExcerpt  *string `json:"sourceExcerpt"`
}

func parseJSON(content []byte) (Draft, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var report jsonReport
	if err := decoder.Decode(&report); err != nil {
		return Draft{}, fmt.Errorf("decode laboratory JSON: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Draft{}, err
	}

	draft := Draft{
		Laboratory:       cleanPointer(report.Laboratory),
		SampleIdentifier: cleanPointer(report.SampleIdentifier),
		CollectionDate:   cleanPointer(report.CollectionDate),
		Matrix:           cleanPointer(report.Matrix),
		Method:           cleanPointer(report.Method),
		Basis:            cleanPointer(report.Basis),
		Analytes:         make([]Analyte, 0, len(report.Analytes)),
	}
	for index, input := range report.Analytes {
		page := input.SourcePage
		if page == 0 {
			page = 1
		}
		excerpt := cleanString(valueOrEmpty(input.SourceExcerpt))
		if excerpt == "" {
			encoded, _ := json.Marshal(input)
			excerpt = string(encoded)
		}
		analyte := analyteFromResult(input.Analyte, input.Result, input.Unit, input.Basis, input.Qualifier, input.ReportingLimit, input.DetectionLimit)
		analyte.SourcePage = page
		analyte.SourceExcerpt = excerpt
		analyte.SourceBounds = &SourceBounds{Start: index, End: index + 1}
		draft.Analytes = append(draft.Analytes, analyte)
	}
	return draft, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("laboratory JSON must contain one object")
		}
		return fmt.Errorf("decode laboratory JSON: %w", err)
	}
	return nil
}

func parseCSV(content []byte) (Draft, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.TrimLeadingSpace = true
	reader.ReuseRecord = false
	rows, err := reader.ReadAll()
	if err != nil {
		return Draft{}, fmt.Errorf("decode laboratory CSV: %w", err)
	}
	if len(rows) < 2 {
		return Draft{}, errors.New("laboratory CSV must contain a header and at least one result row")
	}

	headers := make(map[string]int, len(rows[0]))
	for index, header := range rows[0] {
		headers[normalizeHeader(header)] = index
	}
	analyteColumn := findColumn(headers, "analyte", "compound", "analytename")
	resultColumn := findColumn(headers, "result", "value", "reportedresult")
	if analyteColumn < 0 || resultColumn < 0 {
		return Draft{}, errors.New("laboratory CSV requires analyte and result columns")
	}

	draft := Draft{Analytes: make([]Analyte, 0, len(rows)-1)}
	for rowIndex, row := range rows[1:] {
		if allBlank(row) {
			continue
		}
		if len(row) != len(rows[0]) {
			return Draft{}, fmt.Errorf("laboratory CSV row %d has %d fields; expected %d", rowIndex+2, len(row), len(rows[0]))
		}
		if draft.Laboratory == nil {
			draft.Laboratory = rowPointer(row, headers, "laboratory", "lab")
			draft.SampleIdentifier = rowPointer(row, headers, "sampleidentifier", "sampleid", "sample")
			draft.CollectionDate = rowPointer(row, headers, "collectiondate", "sampledate")
			draft.Matrix = rowPointer(row, headers, "matrix", "samplematrix")
			draft.Method = rowPointer(row, headers, "method", "analyticalmethod")
			draft.Basis = rowPointer(row, headers, "basis", "weightbasis", "drywetbasis")
		}

		raw := strings.Join(row, ",")
		analyte := analyteFromResult(
			row[analyteColumn],
			row[resultColumn],
			rowPointer(row, headers, "unit", "units"),
			rowPointer(row, headers, "basis", "weightbasis", "drywetbasis"),
			rowPointer(row, headers, "qualifier", "flag", "resultqualifier"),
			rowPointer(row, headers, "reportinglimit", "rl", "reportlimit"),
			rowPointer(row, headers, "detectionlimit", "dl", "mdl"),
		)
		analyte.SourcePage = 1
		analyte.SourceExcerpt = raw
		analyte.SourceBounds = &SourceBounds{Start: rowIndex + 1, End: rowIndex + 2}
		draft.Analytes = append(draft.Analytes, analyte)
	}
	return draft, nil
}

var (
	pfosPattern      = regexp.MustCompile(`\bpfos\b`)
	pfoaPattern      = regexp.MustCompile(`\bpfoa\b`)
	metadataPatterns = map[string]*regexp.Regexp{
		"laboratory":       regexp.MustCompile(`(?i)^\s*(?:laboratory|lab)\s*[:\-]\s*(.+?)\s*$`),
		"sampleIdentifier": regexp.MustCompile(`(?i)^\s*(?:sample\s*(?:identifier|id)|sample)\s*[:\-]\s*(.+?)\s*$`),
		"collectionDate":   regexp.MustCompile(`(?i)^\s*(?:collection|sample)\s*date\s*[:\-]\s*(.+?)\s*$`),
		"matrix":           regexp.MustCompile(`(?i)^\s*(?:sample\s*)?matrix\s*[:\-]\s*(.+?)\s*$`),
		"method":           regexp.MustCompile(`(?i)^\s*(?:analytical\s*)?method\s*[:\-]\s*(.+?)\s*$`),
		"basis":            regexp.MustCompile(`(?i)^\s*(?:weight\s*)?basis\s*[:\-]\s*(.+?)\s*$`),
	}
)

func parsePDFPages(pages []Page) Draft {
	draft := Draft{Analytes: make([]Analyte, 0, 2)}
	for _, page := range pages {
		offset := 0
		for _, line := range strings.Split(page.Text, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				offset += len(line) + 1
				continue
			}
			applyPDFMetadata(&draft, trimmed)
			if canonical := canonicalAnalyte(trimmed); canonical != "" {
				analyte := analyteFromPDFLine(canonical, trimmed)
				analyte.SourcePage = page.Number
				analyte.SourceExcerpt = trimmed
				analyte.SourceBounds = &SourceBounds{Start: offset, End: offset + len(line)}
				draft.Analytes = append(draft.Analytes, analyte)
			}
			offset += len(line) + 1
		}
	}
	return draft
}

func applyPDFMetadata(draft *Draft, line string) {
	for field, pattern := range metadataPatterns {
		match := pattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		value := cleanString(match[1])
		if value == "" {
			return
		}
		switch field {
		case "laboratory":
			setIfNil(&draft.Laboratory, value)
		case "sampleIdentifier":
			setIfNil(&draft.SampleIdentifier, value)
		case "collectionDate":
			setIfNil(&draft.CollectionDate, value)
		case "matrix":
			setIfNil(&draft.Matrix, value)
		case "method":
			setIfNil(&draft.Method, value)
		case "basis":
			setIfNil(&draft.Basis, value)
		}
		return
	}
}

var numberToken = regexp.MustCompile(`(?i)(?:^|\s)(ND|<\s*[0-9][0-9,.]*(?:\.[0-9]+)?(?:e[+\-]?\d+)?|[0-9][0-9,.]*(?:\.[0-9]+)?(?:e[+\-]?\d+)?)(?:\s|$)`)
var reportingLimitPattern = regexp.MustCompile(`(?i)\b(?:RL|reporting\s+limit)\s*[:=]?\s*([0-9][0-9,.]*(?:\.[0-9]+)?(?:e[+\-]?\d+)?)\b`)
var detectionLimitPattern = regexp.MustCompile(`(?i)\b(?:DL|MDL|detection\s+limit)\s*[:=]?\s*([0-9][0-9,.]*(?:\.[0-9]+)?(?:e[+\-]?\d+)?)\b`)

func analyteFromPDFLine(canonical, line string) Analyte {
	result := ""
	if match := numberToken.FindStringSubmatch(line); len(match) == 2 {
		result = strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
	}
	unit := unitFromText(line)
	basis := basisFromText(line)
	qualifier := qualifierFromText(line)
	return analyteFromResult(canonical, result, unit, basis, qualifier, captureDecimal(reportingLimitPattern, line), captureDecimal(detectionLimitPattern, line))
}

func captureDecimal(pattern *regexp.Regexp, value string) *string {
	match := pattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return nil
	}
	return &match[1]
}

func analyteFromResult(name, result string, unit, basis, qualifier, reportingLimit, detectionLimit *string) Analyte {
	result = cleanString(result)
	cleanQualifier := cleanPointer(qualifier)
	isNonDetect := strings.EqualFold(result, "ND") || strings.HasPrefix(result, "<") || qualifierMeansNonDetect(cleanQualifier)
	var value *string
	if !isNonDetect && result != "" {
		candidate := result
		if parsed, err := canonicalDecimal(candidate); err == nil {
			value = &parsed
		}
	}
	if isNonDetect && cleanQualifier == nil {
		label := "ND"
		cleanQualifier = &label
	}
	if reportingLimit == nil && strings.HasPrefix(result, "<") {
		candidate := strings.TrimPrefix(result, "<")
		reportingLimit = &candidate
	}
	return Analyte{
		CanonicalAnalyte: canonicalAnalyte(name),
		ReportedAnalyte:  cleanString(name),
		ResultText:       result,
		Value:            value,
		Unit:             canonicalUnitPointer(unit),
		Basis:            canonicalBasisPointer(basis),
		Qualifier:        cleanQualifier,
		IsNonDetect:      isNonDetect,
		ReportingLimit:   cleanPointer(reportingLimit),
		DetectionLimit:   cleanPointer(detectionLimit),
	}
}

func validateDraft(draft Draft, pages []Page, percentSolids *string) (Draft, []Gap) {
	gaps := make([]Gap, 0)
	draft.Laboratory = cleanPointer(draft.Laboratory)
	draft.SampleIdentifier = cleanPointer(draft.SampleIdentifier)
	draft.CollectionDate = cleanPointer(draft.CollectionDate)
	draft.Method = cleanPointer(draft.Method)
	draft.Matrix = canonicalMatrixPointer(draft.Matrix)
	draft.Basis = canonicalBasisPointer(draft.Basis)

	requirePointer(&gaps, "MISSING_LABORATORY", "laboratory", draft.Laboratory, "Laboratory is missing.", "Enter the laboratory shown on the report.")
	requirePointer(&gaps, "MISSING_SAMPLE_IDENTIFIER", "sampleIdentifier", draft.SampleIdentifier, "Sample identifier is missing.", "Enter the sample identifier shown on the report.")
	if draft.CollectionDate == nil {
		addGap(&gaps, "MISSING_COLLECTION_DATE", "collectionDate", "Collection date is missing.", "Enter the collection date shown on the report.")
	} else if _, err := time.Parse("2006-01-02", *draft.CollectionDate); err != nil {
		addGap(&gaps, "INVALID_COLLECTION_DATE", "collectionDate", "Collection date must use YYYY-MM-DD.", "Correct the collection date using the report.")
	}
	if draft.Matrix == nil {
		addGap(&gaps, "MISSING_MATRIX", "matrix", "Sample matrix is missing.", "Confirm that the sample is biosolids or sewage sludge.")
	} else if *draft.Matrix != "BIOSOLIDS" {
		addGap(&gaps, "UNVERIFIED_MATRIX", "matrix", "The sample matrix is not verified as biosolids or sewage sludge.", "Correct the matrix or use a biosolids laboratory result.")
	}
	requirePointer(&gaps, "MISSING_METHOD", "method", draft.Method, "Analytical method is missing.", "Enter the analytical method shown on the report.")
	if draft.Basis == nil {
		addGap(&gaps, "MISSING_BASIS", "basis", "Dry- or wet-weight basis is missing.", "Confirm the weight basis shown on the report.")
	}

	grouped := make(map[string][]Analyte, 2)
	for _, analyte := range draft.Analytes {
		if analyte.CanonicalAnalyte == "" {
			continue
		}
		grouped[analyte.CanonicalAnalyte] = append(grouped[analyte.CanonicalAnalyte], analyte)
	}
	draft.Analytes = draft.Analytes[:0]
	for _, canonical := range []string{"PFOS", "PFOA"} {
		results := grouped[canonical]
		if len(results) == 0 {
			addGap(&gaps, "MISSING_ANALYTE", "analytes."+strings.ToLower(canonical), canonical+" is missing.", "Add the "+canonical+" result exactly as shown on the report.")
			continue
		}
		selected := results[0]
		if len(results) > 1 {
			for _, candidate := range results[1:] {
				if !equivalentAnalyte(selected, candidate) {
					addGap(&gaps, "CONFLICTING_ANALYTE_RESULTS", "analytes."+strings.ToLower(canonical), "Multiple "+canonical+" results conflict.", "Select the result that belongs to this biosolids sample.")
					break
				}
			}
		}
		if selected.Basis == nil {
			selected.Basis = draft.Basis
		}
		validateAnalyte(&selected, percentSolids, pages, &gaps)
		draft.Analytes = append(draft.Analytes, selected)
	}
	return draft, gaps
}

func validateAnalyte(analyte *Analyte, percentSolids *string, pages []Page, gaps *[]Gap) {
	field := "analytes." + strings.ToLower(analyte.CanonicalAnalyte)
	analyte.Unit = canonicalUnitPointer(analyte.Unit)
	analyte.Basis = canonicalBasisPointer(analyte.Basis)
	if strings.TrimSpace(analyte.ResultText) == "" || (!analyte.IsNonDetect && analyte.Value == nil) {
		addGap(gaps, "AMBIGUOUS_RESULT", field+".result", analyte.CanonicalAnalyte+" result is missing or ambiguous.", "Enter the result exactly as shown on the report.")
	}
	if analyte.Unit == nil {
		addGap(gaps, "MISSING_UNIT", field+".unit", analyte.CanonicalAnalyte+" unit is missing.", "Enter the unit shown beside the result.")
	}
	if analyte.Basis == nil {
		addGap(gaps, "MISSING_ANALYTE_BASIS", field+".basis", analyte.CanonicalAnalyte+" weight basis is missing.", "Confirm the result's dry- or wet-weight basis.")
	}
	if analyte.IsNonDetect && analyte.ReportingLimit == nil {
		addGap(gaps, "MISSING_REPORTING_LIMIT", field+".reportingLimit", analyte.CanonicalAnalyte+" is non-detect but has no reporting limit.", "Enter the reporting limit shown on the report.")
	}
	if analyte.SourcePage < 1 || analyte.SourcePage > len(pages) {
		addGap(gaps, "INVALID_SOURCE_PAGE", field+".sourcePage", analyte.CanonicalAnalyte+" does not point to a valid source page.", "Select the page containing this result.")
	}
	if strings.TrimSpace(analyte.SourceExcerpt) == "" {
		addGap(gaps, "MISSING_SOURCE_EXCERPT", field+".sourceExcerpt", analyte.CanonicalAnalyte+" has no source excerpt.", "Select the source text containing this result.")
	}

	var err error
	analyte.Value, err = decimalPointer(analyte.Value)
	if err != nil {
		addGap(gaps, "INVALID_RESULT_NUMBER", field+".result", analyte.CanonicalAnalyte+" result is not a valid non-negative decimal.", "Correct the result using the report.")
	}
	analyte.ReportingLimit, err = decimalPointer(analyte.ReportingLimit)
	if err != nil {
		addGap(gaps, "INVALID_REPORTING_LIMIT", field+".reportingLimit", analyte.CanonicalAnalyte+" reporting limit is not a valid non-negative decimal.", "Correct the reporting limit using the report.")
	}
	analyte.DetectionLimit, err = decimalPointer(analyte.DetectionLimit)
	if err != nil {
		addGap(gaps, "INVALID_DETECTION_LIMIT", field+".detectionLimit", analyte.CanonicalAnalyte+" detection limit is not a valid non-negative decimal.", "Correct the detection limit using the report.")
	}
	if analyte.Unit != nil && analyte.Basis != nil {
		analyte.NormalizedValueUGKGDry, _ = normalizePointer(analyte.Value, *analyte.Unit, *analyte.Basis, percentSolids)
		analyte.NormalizedReportingLimitUGKGDry, _ = normalizePointer(analyte.ReportingLimit, *analyte.Unit, *analyte.Basis, percentSolids)
		analyte.NormalizedDetectionLimitUGKGDry, _ = normalizePointer(analyte.DetectionLimit, *analyte.Unit, *analyte.Basis, percentSolids)
		if *analyte.Unit == "UG_L" {
			addGap(gaps, "UNSUPPORTED_CONCENTRATION_UNIT", field+".unit", analyte.CanonicalAnalyte+" is reported as a liquid concentration.", "Provide a mass-based biosolids result or a reviewed conversion.")
		}
		if *analyte.Basis == "WET" && percentSolids == nil {
			addGap(gaps, "MISSING_PERCENT_SOLIDS", "percentSolids", "Wet-weight results require percent solids for dry-weight conversion.", "Enter percent solids for this batch.")
		}
	}
}

func normalizePointer(value *string, unit, basis string, percentSolids *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	return normalizeConcentration(*value, unit, basis, percentSolids)
}

func equivalentAnalyte(left, right Analyte) bool {
	return left.ResultText == right.ResultText && pointerEqual(left.Unit, right.Unit) && pointerEqual(left.Basis, right.Basis) && pointerEqual(left.Qualifier, right.Qualifier)
}

func pointerEqual(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func addGap(gaps *[]Gap, code, field, detail, resolution string) {
	*gaps = append(*gaps, Gap{Code: code, FieldName: field, Detail: detail, Resolution: resolution, Status: "OPEN"})
}

func requirePointer(gaps *[]Gap, code, field string, value *string, detail, resolution string) {
	if value == nil {
		addGap(gaps, code, field, detail, resolution)
	}
}

func canonicalAnalyte(value string) string {
	normalized := strings.ToLower(value)
	switch {
	case strings.Contains(normalized, "1763-23-1"), strings.Contains(normalized, "perfluorooctanesulfon"), pfosPattern.MatchString(normalized):
		return "PFOS"
	case strings.Contains(normalized, "335-67-1"), strings.Contains(normalized, "perfluorooctanoic"), pfoaPattern.MatchString(normalized):
		return "PFOA"
	default:
		return ""
	}
}

func canonicalUnitPointer(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	normalized = strings.NewReplacer("μ", "u", "µ", "u", " ", "", "per", "/").Replace(normalized)
	var canonical string
	switch normalized {
	case "ug/kg", "ugkg", "mcg/kg", "ug_kg":
		canonical = "UG_KG"
	case "ng/g", "ngg", "ng_g":
		canonical = "NG_G"
	case "mg/kg", "mgkg", "mg_kg":
		canonical = "MG_KG"
	case "ug/l", "ugl", "mcg/l", "ug_l":
		canonical = "UG_L"
	default:
		return nil
	}
	return &canonical
}

func unitFromText(value string) *string {
	pattern := regexp.MustCompile(`(?i)(?:µ|μ|u|mc|n|m)g\s*(?:/|per)\s*(?:kg|g|l)\b`)
	match := pattern.FindString(value)
	if match == "" {
		return nil
	}
	return &match
}

func canonicalBasisPointer(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	var canonical string
	switch {
	case normalized == "dry", normalized == "dw", strings.Contains(normalized, "dry weight"):
		canonical = "DRY"
	case normalized == "wet", normalized == "ww", strings.Contains(normalized, "wet weight"):
		canonical = "WET"
	default:
		return nil
	}
	return &canonical
}

func basisFromText(value string) *string {
	pattern := regexp.MustCompile(`(?i)\b(?:dry|wet)(?:[- ]weight)?\b|\b(?:dw|ww)\b`)
	match := pattern.FindString(value)
	if match == "" {
		return nil
	}
	return &match
}

func qualifierFromText(value string) *string {
	for _, candidate := range []string{"ND", "U", "J", "UJ"} {
		if regexp.MustCompile(`(?i)(?:^|\s)` + candidate + `(?:\s|$)`).MatchString(value) {
			return &candidate
		}
	}
	return nil
}

func qualifierMeansNonDetect(value *string) bool {
	if value == nil {
		return false
	}
	return strings.EqualFold(*value, "ND") || strings.EqualFold(*value, "U") || strings.EqualFold(*value, "UJ")
}

func canonicalMatrixPointer(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	canonical := strings.ToUpper(strings.TrimSpace(*value))
	if strings.Contains(normalized, "biosolid") || strings.Contains(normalized, "sewage sludge") || normalized == "sludge" {
		canonical = "BIOSOLIDS"
	}
	return &canonical
}

func normalizeHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "")
}

func findColumn(headers map[string]int, names ...string) int {
	for _, name := range names {
		if index, ok := headers[name]; ok {
			return index
		}
	}
	return -1
}

func rowPointer(row []string, headers map[string]int, names ...string) *string {
	index := findColumn(headers, names...)
	if index < 0 {
		return nil
	}
	value := cleanString(row[index])
	if value == "" {
		return nil
	}
	return &value
}

func cleanPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := cleanString(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

func cleanString(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func setIfNil(target **string, value string) {
	if *target == nil {
		*target = &value
	}
}

func allBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
