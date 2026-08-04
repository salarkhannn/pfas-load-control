package mireye

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type FieldsSummary struct {
	CatalogVersion string   `json:"catalogVersion"`
	FieldCount     int      `json:"fieldCount"`
	PresetCount    int      `json:"presetCount"`
	Layers         []string `json:"layers"`
}

type PlansSummary struct {
	PlanCount int      `json:"planCount"`
	PlanNames []string `json:"planNames"`
	HasCosts  bool     `json:"hasCreditCosts"`
}

type UsageSummary struct {
	PlanName         string      `json:"planName"`
	CreditsUsed      json.Number `json:"creditsUsed"`
	CreditsIncluded  json.Number `json:"creditsIncluded"`
	CreditsRemaining json.Number `json:"creditsRemaining"`
	ResetsAt         string      `json:"resetsAt"`
}

func validateFields(body []byte) (any, error) {
	var payload struct {
		Fields []struct {
			Name   string `json:"name"`
			Layer  string `json:"layer"`
			Source string `json:"source"`
		} `json:"fields"`
		Presets    map[string]json.RawMessage `json:"presets"`
		USEnvelope map[string]json.RawMessage `json:"us_envelope"`
		Version    string                     `json:"version"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("mireye field catalog was not valid JSON")
	}
	if payload.Version == "" || len(payload.Fields) == 0 || payload.Presets == nil || payload.USEnvelope == nil {
		return nil, errors.New("mireye field catalog omitted required catalog metadata")
	}
	layerSet := make(map[string]struct{})
	for index, field := range payload.Fields {
		if field.Name == "" || field.Layer == "" || field.Source == "" {
			return nil, fmt.Errorf("mireye field catalog entry %d omitted required provenance", index)
		}
		layerSet[field.Layer] = struct{}{}
	}
	layers := make([]string, 0, len(layerSet))
	for layer := range layerSet {
		layers = append(layers, layer)
	}
	sort.Strings(layers)
	return FieldsSummary{CatalogVersion: payload.Version, FieldCount: len(payload.Fields), PresetCount: len(payload.Presets), Layers: layers}, nil
}

func validatePlans(body []byte) (any, error) {
	var payload struct {
		Plans []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"plans"`
		Credits struct {
			Costs map[string]json.RawMessage `json:"costs"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("mireye plan catalog was not valid JSON")
	}
	if len(payload.Plans) == 0 || payload.Credits.Costs == nil {
		return nil, errors.New("mireye plan catalog omitted required plan or cost metadata")
	}
	names := make([]string, 0, len(payload.Plans))
	for index, plan := range payload.Plans {
		if plan.ID == "" || plan.Name == "" {
			return nil, fmt.Errorf("mireye plan catalog entry %d omitted an identifier", index)
		}
		names = append(names, plan.Name)
	}
	return PlansSummary{PlanCount: len(payload.Plans), PlanNames: names, HasCosts: len(payload.Credits.Costs) > 0}, nil
}

func validateUsage(body []byte) (any, error) {
	var payload struct {
		Period struct {
			ResetsAt string `json:"resets_at"`
		} `json:"period"`
		Plan struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"plan"`
		Credits struct {
			Used      json.Number `json:"used"`
			Included  json.Number `json:"included"`
			Remaining json.Number `json:"remaining"`
		} `json:"credits"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, errors.New("mireye usage response was not valid JSON")
	}
	if payload.Plan.ID == "" || payload.Plan.Name == "" || payload.Period.ResetsAt == "" || payload.Credits.Used == "" || payload.Credits.Included == "" || payload.Credits.Remaining == "" {
		return nil, errors.New("mireye usage response omitted required account metadata")
	}
	return UsageSummary{
		PlanName:         payload.Plan.Name,
		CreditsUsed:      payload.Credits.Used,
		CreditsIncluded:  payload.Credits.Included,
		CreditsRemaining: payload.Credits.Remaining,
		ResetsAt:         payload.Period.ResetsAt,
	}, nil
}
