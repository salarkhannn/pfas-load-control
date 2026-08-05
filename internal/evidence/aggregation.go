package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

type sampleObservation struct {
	Index     int
	Status    string
	Value     json.RawMessage
	Unit      string
	Source    string
	SourceURL string
	FetchedAt *time.Time
}

type aggregateResult struct {
	State         string
	Value         json.RawMessage
	Unit          string
	Source        string
	SourceURL     string
	FetchedAt     *time.Time
	OKCount       int
	AbsentCount   int
	FailedCount   int
	SampleIndices []int
}

func aggregateField(spec fieldSpec, observations []sampleObservation, sampleCount int) (aggregateResult, error) {
	result := aggregateResult{SampleIndices: []int{}}
	ok := make([]sampleObservation, 0, len(observations))
	for _, observation := range observations {
		switch observation.Status {
		case "ok":
			ok = append(ok, observation)
			result.OKCount++
			result.SampleIndices = append(result.SampleIndices, observation.Index)
		case "absent":
			result.AbsentCount++
		case "failed":
			result.FailedCount++
		default:
			return aggregateResult{}, fmt.Errorf("unknown sample status %q", observation.Status)
		}
	}
	missing := sampleCount - len(observations)
	if missing > 0 {
		result.FailedCount += missing
	}
	if result.OKCount == 0 {
		result.State = "UNAVAILABLE"
		return result, nil
	}
	result.State = "COMPLETE"
	if result.OKCount != sampleCount || result.AbsentCount > 0 || result.FailedCount > 0 {
		result.State = "PARTIAL"
	}
	result.Unit, result.Source, result.SourceURL, result.FetchedAt = sharedProvenance(ok)

	var value any
	var err error
	switch spec.Method {
	case aggregateAnyTrue:
		value, err = anyTrue(ok)
	case aggregateMinimum:
		value, err = numericBoundary(ok, true)
	case aggregateMaximum:
		value, err = numericBoundary(ok, false)
	case aggregateNumericRange:
		value, err = numericRange(ok)
	case aggregateDistribution:
		value, err = distribution(ok)
	default:
		err = errors.New("unsupported aggregation method")
	}
	if err != nil {
		return aggregateResult{}, fmt.Errorf("aggregate %s: %w", spec.Name, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return aggregateResult{}, fmt.Errorf("encode %s aggregate: %w", spec.Name, err)
	}
	result.Value = encoded
	return result, nil
}

func anyTrue(observations []sampleObservation) (bool, error) {
	result := false
	for _, observation := range observations {
		var value bool
		if err := json.Unmarshal(observation.Value, &value); err != nil {
			return false, errors.New("expected a boolean value")
		}
		result = result || value
	}
	return result, nil
}

func numericBoundary(observations []sampleObservation, minimum bool) (float64, error) {
	values, err := numericValues(observations)
	if err != nil {
		return 0, err
	}
	sort.Float64s(values)
	if minimum {
		return values[0], nil
	}
	return values[len(values)-1], nil
}

func numericRange(observations []sampleObservation) (map[string]float64, error) {
	values, err := numericValues(observations)
	if err != nil {
		return nil, err
	}
	sort.Float64s(values)
	middle := len(values) / 2
	median := values[middle]
	if len(values)%2 == 0 {
		median = (values[middle-1] + values[middle]) / 2
	}
	return map[string]float64{"min": values[0], "max": values[len(values)-1], "median": median}, nil
}

func numericValues(observations []sampleObservation) ([]float64, error) {
	values := make([]float64, 0, len(observations))
	for _, observation := range observations {
		decoder := json.NewDecoder(bytes.NewReader(observation.Value))
		decoder.UseNumber()
		var decoded any
		if err := decoder.Decode(&decoded); err != nil {
			return nil, errors.New("expected a numeric value")
		}
		number, ok := decoded.(json.Number)
		if !ok {
			return nil, errors.New("expected a numeric value")
		}
		value, err := number.Float64()
		if err != nil {
			return nil, errors.New("expected a finite numeric value")
		}
		values = append(values, value)
	}
	return values, nil
}

type distributionValue struct {
	Value json.RawMessage `json:"value"`
	Count int             `json:"count"`
}

func distribution(observations []sampleObservation) ([]distributionValue, error) {
	counts := make(map[string]int)
	values := make(map[string]json.RawMessage)
	for _, observation := range observations {
		if !json.Valid(observation.Value) || bytes.Equal(bytes.TrimSpace(observation.Value), []byte("null")) {
			return nil, errors.New("expected a non-null category")
		}
		key := string(observation.Value)
		counts[key]++
		values[key] = observation.Value
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]distributionValue, 0, len(keys))
	for _, key := range keys {
		result = append(result, distributionValue{Value: values[key], Count: counts[key]})
	}
	return result, nil
}

func sharedProvenance(observations []sampleObservation) (string, string, string, *time.Time) {
	unit, source, sourceURL := observations[0].Unit, observations[0].Source, observations[0].SourceURL
	var fetchedAt *time.Time
	for _, observation := range observations {
		if observation.Unit != unit {
			unit = ""
		}
		if observation.Source != source || observation.SourceURL != sourceURL {
			source, sourceURL = "Multiple cited sources", ""
		}
		if observation.FetchedAt != nil && (fetchedAt == nil || observation.FetchedAt.After(*fetchedAt)) {
			value := *observation.FetchedAt
			fetchedAt = &value
		}
	}
	return unit, source, sourceURL, fetchedAt
}
