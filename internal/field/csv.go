package field

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

func (s *Service) ImportCSV(ctx context.Context, workspaceKey, facilityID string, content []byte) (Import, error) {
	if len(content) == 0 || len(content) > MaxCSVBytes {
		return Import{}, fmt.Errorf("%w: CSV file must be no larger than 1 MiB", ErrInvalid)
	}
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return Import{}, fmt.Errorf("%w: CSV header could not be read", ErrInvalid)
	}
	columns := make(map[string]int, len(header))
	for index, column := range header {
		columns[strings.ToLower(strings.TrimSpace(column))] = index
	}
	for _, required := range []string{"name", "locator_kind"} {
		if _, ok := columns[required]; !ok {
			return Import{}, fmt.Errorf("%w: CSV must contain %s and locator_kind columns", ErrInvalid, required)
		}
	}
	result := Import{Results: []CSVResult{}}
	for rowNumber := 2; ; rowNumber++ {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Import{}, fmt.Errorf("%w: CSV row %d could not be read", ErrInvalid, rowNumber)
		}
		if len(result.Results) >= MaxImportRows {
			return Import{}, fmt.Errorf("%w: CSV can contain at most %d fields", ErrInvalid, MaxImportRows)
		}
		value := func(name string) string {
			index, ok := columns[name]
			if !ok || index >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[index])
		}
		kind := LocatorKind(strings.ToUpper(value("locator_kind")))
		created, createErr := s.Create(ctx, workspaceKey, facilityID, CreateInput{
			Name: value("name"), LocatorKind: kind, Locator: value("locator"),
			County: value("county"), GeoJSON: value("geojson"),
		})
		item := CSVResult{Row: rowNumber}
		if createErr != nil {
			item.Problem = publicError(createErr)
		} else {
			item.Field = &created
		}
		result.Results = append(result.Results, item)
	}
	if len(result.Results) == 0 {
		return Import{}, fmt.Errorf("%w: CSV must contain at least one field", ErrInvalid)
	}
	return result, nil
}

func publicError(err error) string {
	message := err.Error()
	for _, sentinel := range []error{ErrInvalid, ErrConflict, ErrNotFound, ErrExternal} {
		message = strings.TrimSpace(strings.TrimPrefix(message, sentinel.Error()+":"))
	}
	if message == "" {
		return "The field could not be imported."
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if strings.HasSuffix(message, ".") {
		return message
	}
	return message + "."
}
