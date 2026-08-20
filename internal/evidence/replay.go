package evidence

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/salarkhannn/pfas-load-control/internal/mireye"
)

// AggregateFrozenFact applies the same catalog and whole-field aggregation used by
// live Mireye evaluations to a deterministic set of provider observations.
func AggregateFrozenFact(name string, values []json.RawMessage, source, sourceURL string, retrievedAt time.Time) (FieldFact, error) {
	var spec *fieldSpec
	for index := range physicalFieldSpecs {
		if physicalFieldSpecs[index].Name == name {
			spec = &physicalFieldSpecs[index]
			break
		}
	}
	if spec == nil {
		return FieldFact{}, fmt.Errorf("unknown physical field %q", name)
	}
	observations := make([]sampleObservation, 0, len(values))
	for index, value := range values {
		observations = append(observations, sampleObservation{Index: index, Status: "ok", Value: value, Source: source, SourceURL: sourceURL, FetchedAt: &retrievedAt})
	}
	result, err := aggregateField(*spec, observations, len(values))
	if err != nil {
		return FieldFact{}, err
	}
	return FieldFact{
		Name: spec.Name, Label: spec.Label, Category: spec.Category, State: result.State,
		AggregateMethod: string(spec.Method), Value: result.Value, Unit: result.Unit,
		Source: result.Source, SourceURL: result.SourceURL, FetchedAt: result.FetchedAt,
		OKCount: result.OKCount, AbsentCount: result.AbsentCount, FailedCount: result.FailedCount,
		Critical: spec.Critical, Samples: []SampleFact{},
	}, nil
}

// AggregateFetchBatchFact maps a captured FetchBatch result through the same
// field normalization and whole-field aggregation used by live evaluations.
// Provider names, source URLs, timestamps, unavailable values, and failures are
// preserved per sample instead of being assigned by the replay caller.
func AggregateFetchBatchFact(batch mireye.FetchBatchResult, name string) (FieldFact, error) {
	var spec *fieldSpec
	for index := range physicalFieldSpecs {
		if physicalFieldSpecs[index].Name == name {
			spec = &physicalFieldSpecs[index]
			break
		}
	}
	if spec == nil {
		return FieldFact{}, fmt.Errorf("unknown physical field %q", name)
	}
	observations := make([]sampleObservation, 0, len(batch.Results))
	samples := make([]SampleFact, 0, len(batch.Results))
	for index, item := range batch.Results {
		fact := normalizedField(item, name)
		observation := sampleObservation{Index: index, Status: fact.Status, Value: fact.Value}
		if fact.Unit != nil {
			observation.Unit = *fact.Unit
		}
		if fact.Source != nil {
			observation.Source = *fact.Source
		}
		if fact.SourceURL != nil {
			observation.SourceURL = *fact.SourceURL
		}
		observation.FetchedAt = fact.FetchedAt
		observations = append(observations, observation)
		sample := SampleFact{Index: index, Label: fmt.Sprintf("Captured sample %d", index+1), Status: fact.Status, Value: append(json.RawMessage(nil), fact.Value...), Unit: observation.Unit, Source: observation.Source, SourceURL: observation.SourceURL, FetchedAt: fact.FetchedAt}
		if item.Latitude != nil {
			sample.Latitude = *item.Latitude
		}
		if item.Longitude != nil {
			sample.Longitude = *item.Longitude
		}
		if fact.Confidence != nil {
			sample.Confidence = *fact.Confidence
		}
		if fact.DatasetVintage != nil {
			sample.DatasetVintage = *fact.DatasetVintage
		}
		if fact.Notes != nil {
			sample.Notes = *fact.Notes
		}
		if fact.Error != nil {
			sample.Error = *fact.Error
		}
		samples = append(samples, sample)
	}
	result, err := aggregateField(*spec, observations, len(batch.Results))
	if err != nil {
		return FieldFact{}, err
	}
	return FieldFact{Name: spec.Name, Label: spec.Label, Category: spec.Category, State: result.State, AggregateMethod: string(spec.Method), Value: result.Value, Unit: result.Unit, Source: result.Source, SourceURL: result.SourceURL, FetchedAt: result.FetchedAt, OKCount: result.OKCount, AbsentCount: result.AbsentCount, FailedCount: result.FailedCount, Critical: spec.Critical, Samples: samples}, nil
}
