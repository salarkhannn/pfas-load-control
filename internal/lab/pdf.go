package lab

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxToolOutput = 8 * 1024 * 1024
	maxToolError  = 64 * 1024
)

type ExtractionError struct {
	Code   string
	Detail string
}

func (e *ExtractionError) Error() string {
	return e.Detail
}

type SystemPDFExtractor struct {
	pdfInfo   string
	pdfText   string
	pdfToPPM  string
	tesseract string
}

func NewSystemPDFExtractor() *SystemPDFExtractor {
	return &SystemPDFExtractor{
		pdfInfo:   "pdfinfo",
		pdfText:   "pdftotext",
		pdfToPPM:  "pdftoppm",
		tesseract: "tesseract",
	}
}

func (e *SystemPDFExtractor) Extract(ctx context.Context, content []byte) ([]Page, error) {
	directory, err := os.MkdirTemp("", "pfas-lab-*")
	if err != nil {
		return nil, fmt.Errorf("create private extraction directory: %w", err)
	}
	defer os.RemoveAll(directory)
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure extraction directory: %w", err)
	}

	inputPath := filepath.Join(directory, "report.pdf")
	if err := os.WriteFile(inputPath, content, 0o600); err != nil {
		return nil, fmt.Errorf("stage PDF for extraction: %w", err)
	}
	info, err := runBounded(ctx, e.pdfInfo, inputPath)
	if err != nil {
		return nil, toolError("INVALID_PDF", "The uploaded PDF could not be opened.", err)
	}
	pageCount, err := parsePageCount(info)
	if err != nil {
		return nil, err
	}
	if pageCount > MaxPDFPages {
		return nil, &ExtractionError{Code: "TOO_MANY_PAGES", Detail: fmt.Sprintf("The PDF has %d pages; the limit is %d.", pageCount, MaxPDFPages)}
	}

	bbox, err := runBounded(ctx, e.pdfText, "-bbox-layout", "-enc", "UTF-8", inputPath, "-")
	if err != nil {
		return nil, toolError("PDF_TEXT_EXTRACTION_FAILED", "Text could not be extracted from the PDF.", err)
	}
	pages, err := parseBBoxPages(bbox)
	if err != nil {
		return nil, &ExtractionError{Code: "PDF_TEXT_EXTRACTION_FAILED", Detail: "The PDF text layer could not be read safely."}
	}
	for len(pages) < pageCount {
		pages = append(pages, Page{Number: len(pages) + 1, ExtractionMethod: "PDF_TEXT"})
	}
	layout, err := runBounded(ctx, e.pdfText, "-layout", "-enc", "UTF-8", inputPath, "-")
	if err != nil {
		return nil, toolError("PDF_TEXT_EXTRACTION_FAILED", "Text could not be extracted from the PDF.", err)
	}
	applyLayoutText(pages, layout)

	for index := range pages {
		if usefulText(pages[index].Text) >= 20 {
			continue
		}
		text, ocrErr := e.ocrPage(ctx, inputPath, directory, pages[index].Number)
		if ocrErr != nil {
			var extractionErr *ExtractionError
			if errors.As(ocrErr, &extractionErr) && ocrFailureRecoverable(extractionErr.Code) {
				if !anyReadablePage(pages) {
					return nil, ocrErr
				}
				pages[index].ReadError = extractionErr.Detail
				continue
			}
			return nil, ocrErr
		}
		pages[index].Text = text
		pages[index].ExtractionMethod = "OCR"
	}
	return pages, nil
}

func ocrFailureRecoverable(code string) bool {
	switch code {
	case "OCR_UNAVAILABLE", "OCR_RENDER_FAILED", "OCR_FAILED":
		return true
	}
	return false
}

func anyReadablePage(pages []Page) bool {
	for _, page := range pages {
		if usefulText(page.Text) >= 20 {
			return true
		}
	}
	return false
}

func applyLayoutText(pages []Page, data []byte) {
	textPages := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\f")
	for index := range pages {
		if index >= len(textPages) {
			return
		}
		pages[index].Text = strings.Trim(textPages[index], "\r\n")
	}
}

func (e *SystemPDFExtractor) ocrPage(ctx context.Context, inputPath, directory string, page int) (string, error) {
	if _, err := exec.LookPath(e.tesseract); err != nil {
		return "", &ExtractionError{Code: "OCR_UNAVAILABLE", Detail: "This scanned PDF needs OCR, but OCR is not available on the server."}
	}
	prefix := filepath.Join(directory, fmt.Sprintf("page-%02d", page))
	if _, err := runBounded(ctx, e.pdfToPPM, "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-r", "240", "-png", "-singlefile", inputPath, prefix); err != nil {
		return "", toolError("OCR_RENDER_FAILED", "A scanned PDF page could not be prepared for OCR.", err)
	}
	tsv, err := runBounded(ctx, e.tesseract, prefix+".png", "stdout", "-l", "eng", "--psm", "6", "tsv")
	if err != nil {
		return "", toolError("OCR_FAILED", "Text could not be read from a scanned PDF page.", err)
	}
	text, err := parseTesseractTSV(tsv)
	if err != nil {
		return "", &ExtractionError{Code: "OCR_FAILED", Detail: "OCR output could not be read safely."}
	}
	return text, nil
}

var pageCountPattern = regexp.MustCompile(`(?m)^Pages:\s+(\d+)\s*$`)

func parsePageCount(output []byte) (int, error) {
	match := pageCountPattern.FindSubmatch(output)
	if len(match) != 2 {
		return 0, &ExtractionError{Code: "INVALID_PDF", Detail: "The PDF page count could not be verified."}
	}
	count, err := strconv.Atoi(string(match[1]))
	if err != nil || count < 1 {
		return 0, &ExtractionError{Code: "INVALID_PDF", Detail: "The PDF does not contain a readable page."}
	}
	return count, nil
}

func parseBBoxPages(data []byte) ([]Page, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	pages := make([]Page, 0)
	var (
		page       *Page
		lineWords  []string
		insideWord bool
		word       strings.Builder
	)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "page":
				current := Page{Number: len(pages) + 1, ExtractionMethod: "PDF_TEXT"}
				for _, attribute := range value.Attr {
					text := attribute.Value
					switch attribute.Name.Local {
					case "width":
						current.Width = &text
					case "height":
						current.Height = &text
					}
				}
				page = &current
			case "word":
				insideWord = true
				word.Reset()
			}
		case xml.CharData:
			if insideWord {
				word.Write([]byte(value))
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "word":
				insideWord = false
				if text := strings.TrimSpace(word.String()); text != "" {
					lineWords = append(lineWords, text)
				}
			case "line":
				if page != nil && len(lineWords) > 0 {
					if page.Text != "" {
						page.Text += "\n"
					}
					page.Text += strings.Join(lineWords, " ")
				}
				lineWords = lineWords[:0]
			case "page":
				if page != nil {
					pages = append(pages, *page)
					page = nil
				}
			}
		}
	}
	return pages, nil
}

type ocrLineKey struct {
	page  int
	block int
	para  int
	line  int
}

func parseTesseractTSV(data []byte) (string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	rows, err := reader.ReadAll()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", errors.New("empty OCR output")
	}
	lines := make(map[ocrLineKey][]string)
	keys := make([]ocrLineKey, 0)
	for _, row := range rows[1:] {
		if len(row) < 12 || row[0] != "5" || strings.TrimSpace(row[11]) == "" {
			continue
		}
		key := ocrLineKey{page: integer(row[1]), block: integer(row[2]), para: integer(row[3]), line: integer(row[4])}
		if _, exists := lines[key]; !exists {
			keys = append(keys, key)
		}
		lines[key] = append(lines[key], strings.TrimSpace(row[11]))
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].page != keys[j].page {
			return keys[i].page < keys[j].page
		}
		if keys[i].block != keys[j].block {
			return keys[i].block < keys[j].block
		}
		if keys[i].para != keys[j].para {
			return keys[i].para < keys[j].para
		}
		return keys[i].line < keys[j].line
	})
	output := make([]string, 0, len(keys))
	for _, key := range keys {
		output = append(output, strings.Join(lines[key], " "))
	}
	return strings.Join(output, "\n"), nil
}

func usefulText(value string) int {
	count := 0
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			count++
		}
	}
	return count
}

func integer(value string) int {
	number, _ := strconv.Atoi(value)
	return number
}

type cappedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (w *cappedBuffer) Write(data []byte) (int, error) {
	remaining := w.limit - w.buffer.Len()
	if remaining <= 0 {
		return 0, errors.New("command output exceeded limit")
	}
	if len(data) > remaining {
		_, _ = w.buffer.Write(data[:remaining])
		return remaining, errors.New("command output exceeded limit")
	}
	return w.buffer.Write(data)
}

func runBounded(ctx context.Context, name string, args ...string) ([]byte, error) {
	stdout := &cappedBuffer{limit: maxToolOutput}
	stderr := &cappedBuffer{limit: maxToolError}
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s failed: %w", filepath.Base(name), err)
	}
	return stdout.buffer.Bytes(), nil
}

func toolError(code, detail string, cause error) error {
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		return cause
	}
	return &ExtractionError{Code: code, Detail: detail}
}
