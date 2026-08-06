package placement

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
)

var errInvalidNumber = errors.New("invalid decimal value")

func Evaluate(input Input) (PlacementPlan, error) {
	result := PlacementPlan{
		Tier: input.Tier, ConfigVersion: ConfigVersion, ConfigChecksum: ConfigChecksum,
		WetMassKg: input.WetMassKg, PercentSolids: input.PercentSolids,
		Fields: []PlacementField{}, Allocations: []PlacementAllocation{}, Gaps: []PlacementGap{},
	}

	if input.Tier == "PROHIBITED" {
		result.Status = StatusLandApplicationBlocked
		result.Gaps = append(result.Gaps, PlacementGap{Code: "LAND_APPLICATION_PROHIBITED", Detail: "Michigan's active PFAS rule blocks land application for this batch.", Resolution: "Use the alternative-management path."})
		if input.WetMassKg != "" && input.PercentSolids != "" {
			dryTons, err := dryTons(input.WetMassKg, input.PercentSolids)
			if err != nil {
				return PlacementPlan{}, err
			}
			result.BatchDryTons = formatRat(dryTons)
			result.AllocatedDryTons = "0"
			result.UnallocatedDryTons = result.BatchDryTons
		}
		return result, nil
	}

	var batchDryTons *big.Rat
	if input.WetMassKg == "" || input.PercentSolids == "" {
		result.Gaps = append(result.Gaps, PlacementGap{Code: "BATCH_QUANTITY_MISSING", Detail: "Wet mass and total solids are needed to calculate the batch's dry tonnage.", Resolution: "Enter the measured wet mass and total-solids percentage."})
	} else {
		value, err := dryTons(input.WetMassKg, input.PercentSolids)
		if err != nil {
			return PlacementPlan{}, err
		}
		batchDryTons, err = decimal(formatRat(value))
		if err != nil {
			return PlacementPlan{}, err
		}
		result.BatchDryTons = formatRat(value)
	}

	for _, field := range input.Fields {
		fieldResult, err := evaluateField(input.Tier, input.PolicyRate, field)
		if err != nil {
			return PlacementPlan{}, fmt.Errorf("evaluate field %s: %w", field.Name, err)
		}
		result.Fields = append(result.Fields, fieldResult)
	}
	rankFields(result.Fields)
	addCounterfactuals(result.Fields)

	if batchDryTons == nil {
		result.Status = StatusReviewRequired
		return result, nil
	}
	remaining := new(big.Rat).Set(batchDryTons)
	allocated := new(big.Rat)
	for _, field := range result.Fields {
		if field.Disposition != DispositionEligible || field.AvailableCapacity == "" || field.AllowedRate == "" || remaining.Sign() <= 0 {
			continue
		}
		capacity, err := decimal(field.AvailableCapacity)
		if err != nil {
			return PlacementPlan{}, err
		}
		rate, err := decimal(field.AllowedRate)
		if err != nil {
			return PlacementPlan{}, err
		}
		amount := minRat(remaining, capacity)
		if amount.Sign() <= 0 {
			continue
		}
		acres := new(big.Rat).Quo(new(big.Rat).Set(amount), rate)
		result.Allocations = append(result.Allocations, PlacementAllocation{
			Position: len(result.Allocations) + 1, FieldID: field.FieldID, FieldName: field.FieldName,
			DryTons: formatRat(amount), Acres: formatRat(acres), Rate: field.AllowedRate,
		})
		allocated.Add(allocated, amount)
		remaining.Sub(remaining, amount)
	}
	result.AllocatedDryTons = formatRat(allocated)
	result.UnallocatedDryTons = formatRat(remaining)

	if remaining.Sign() > 0 {
		if hasDisposition(result.Fields, DispositionReviewRequired) {
			result.Status = StatusReviewRequired
			detail := formatRat(remaining) + " dry tons remain because one or more fields cannot be used until their review is complete."
			if allocated.Sign() == 0 {
				detail = "No field is currently eligible to receive this batch."
			}
			result.Gaps = append(result.Gaps, PlacementGap{
				Code:       "FIELD_REVIEW_REQUIRED",
				Detail:     detail,
				Resolution: "Resolve the listed field evidence, or add another approved field, then compare again.",
			})
		} else {
			result.Status = StatusInsufficientCapacity
			result.Gaps = append(result.Gaps, PlacementGap{Code: "INSUFFICIENT_ELIGIBLE_CAPACITY", Detail: formatRat(remaining) + " dry tons remain after all eligible field capacity is used.", Resolution: "Add another approved field, or reduce the batch assigned to this plan."})
		}
	} else {
		result.Status = StatusReady
	}
	return result, nil
}

func hasDisposition(fields []PlacementField, disposition Disposition) bool {
	for _, field := range fields {
		if field.Disposition == disposition {
			return true
		}
	}
	return false
}

func evaluateField(tier, policyRate string, input FieldInput) (PlacementField, error) {
	result := PlacementField{
		FieldID: input.ID, FieldName: input.Name, Disposition: DispositionEligible,
		PhysicalEvaluationID: input.PhysicalEvaluationID, Reasons: []string{}, Categories: []VulnerabilityCategory{},
	}
	facts := factMap(input.Facts)
	result.Categories = []VulnerabilityCategory{
		waterCategory(facts), subsurfaceCategory(facts), surfaceCategory(facts),
		humanCategory(facts, input.CropOrUse), uncertaintyCategory(input),
	}
	for _, category := range result.Categories {
		switch category.Band {
		case BandHigh:
			result.HighConcernCount++
		case BandModerate:
			result.ModerateConcernCount++
		case BandUnknown:
			result.DataGapCount++
		}
	}
	result.DataGapCount += input.PhysicalCriticalGaps + input.PhysicalOtherGaps

	switch {
	case input.Status != "READY" || !input.RMPApproved:
		result.Disposition = DispositionIneligible
		result.Reasons = append(result.Reasons, "The field is not confirmed as ready under its approved RMP record.")
	case input.PhysicalEvaluationID == "":
		result.Disposition = DispositionReviewRequired
		result.Reasons = append(result.Reasons, "Physical conditions have not been checked for the confirmed boundary.")
	case input.PhysicalStatus != "SUCCEEDED" || input.PhysicalCriticalGaps > 0:
		result.Disposition = DispositionReviewRequired
		result.Reasons = append(result.Reasons, "Critical physical evidence is incomplete or needs human review.")
	case boolFact(facts["intersects_wetland"]) || boolFact(facts["intersects_nhd_area"]):
		result.Disposition = DispositionReviewRequired
		result.Reasons = append(result.Reasons, "The mapped application boundary overlaps a wetland or surface-water feature and must be reconciled before allocation.")
	}

	if input.UsableAcres == "" || input.AgronomicRate == "" || input.PriorLoadingDryTons == "" {
		if result.Disposition == DispositionEligible {
			result.Disposition = DispositionReviewRequired
		}
		result.Reasons = append(result.Reasons, "Usable acres, the controlling agronomic rate, and prior loading are all required.")
		result.DataGapCount++
	}
	if result.Disposition == DispositionEligible {
		usable, err := positiveDecimal(input.UsableAcres)
		if err != nil {
			return PlacementField{}, err
		}
		agronomic, err := positiveDecimal(input.AgronomicRate)
		if err != nil {
			return PlacementField{}, err
		}
		prior, err := nonnegativeDecimal(input.PriorLoadingDryTons)
		if err != nil {
			return PlacementField{}, err
		}
		allowed := new(big.Rat).Set(agronomic)
		if tier == "ELEVATED" {
			cap, err := positiveDecimal(policyRate)
			if err != nil {
				return PlacementField{}, errors.New("the elevated PFAS rate ceiling is unavailable")
			}
			allowed = minRat(allowed, cap)
		}
		capacity := new(big.Rat).Mul(usable, allowed)
		capacity.Sub(capacity, prior)
		if capacity.Sign() < 0 {
			capacity = new(big.Rat)
		}
		result.AllowedRate = formatRat(allowed)
		result.AvailableCapacity = formatRat(capacity)
		if capacity.Sign() == 0 {
			result.Disposition = DispositionIneligible
			result.Reasons = append(result.Reasons, "No application capacity remains at the controlling rate.")
		}
	}
	if road, ok := numericFact(facts["nearest_road_distance_m"]); ok {
		result.RoadAccessDistanceM = &road
	}
	if len(result.Reasons) == 0 {
		result.Reasons = append(result.Reasons, "Required field and evidence gates are complete.")
	}
	result.Explanation = result.Reasons[0]
	return result, nil
}

func rankFields(fields []PlacementField) {
	sort.SliceStable(fields, func(i, j int) bool {
		left, right := fields[i], fields[j]
		if dispositionOrder(left.Disposition) != dispositionOrder(right.Disposition) {
			return dispositionOrder(left.Disposition) < dispositionOrder(right.Disposition)
		}
		if left.HighConcernCount != right.HighConcernCount {
			return left.HighConcernCount < right.HighConcernCount
		}
		if left.ModerateConcernCount != right.ModerateConcernCount {
			return left.ModerateConcernCount < right.ModerateConcernCount
		}
		if left.DataGapCount != right.DataGapCount {
			return left.DataGapCount < right.DataGapCount
		}
		leftCapacity, _ := decimalOrZero(left.AvailableCapacity)
		rightCapacity, _ := decimalOrZero(right.AvailableCapacity)
		if comparison := leftCapacity.Cmp(rightCapacity); comparison != 0 {
			return comparison > 0
		}
		return roadDistance(left.RoadAccessDistanceM) < roadDistance(right.RoadAccessDistanceM)
	})
	rank := 0
	for index := range fields {
		if fields[index].Disposition != DispositionEligible {
			continue
		}
		rank++
		fields[index].Rank = &rank
	}
}

func addCounterfactuals(fields []PlacementField) {
	var preferred *PlacementField
	for index := range fields {
		if fields[index].Disposition == DispositionEligible {
			preferred = &fields[index]
			break
		}
	}
	if preferred == nil {
		return
	}
	for index := range fields {
		field := &fields[index]
		if field.FieldID == preferred.FieldID {
			field.Counterfactual = "This field is preferred under the current confirmed evidence."
			continue
		}
		switch {
		case field.Disposition != DispositionEligible:
			field.Counterfactual = "Resolve its blocking eligibility or evidence issue before it can outrank an eligible field."
		case field.HighConcernCount > preferred.HighConcernCount:
			field.Counterfactual = "It would rank higher with fewer high-concern vulnerability categories."
		case field.ModerateConcernCount > preferred.ModerateConcernCount:
			field.Counterfactual = "It would rank higher with fewer moderate-concern vulnerability categories."
		case field.DataGapCount > preferred.DataGapCount:
			field.Counterfactual = "It would rank higher after its additional evidence gaps are resolved."
		default:
			field.Counterfactual = "It would rank higher with more usable capacity or easier verified road access."
		}
	}
}

func waterCategory(facts map[string]FactInput) VulnerabilityCategory {
	components := components(facts, "within_floodplain_polygon", "intersects_wetland", "nearest_wetland_distance_m", "wetlands_within_100m_count", "wetlands_within_500m_count", "intersects_nhd_area", "nearest_waterbody_name")
	band, explanation := BandLow, "No direct mapped water overlap or wetland proximity signal was returned."
	if boolFact(facts["within_floodplain_polygon"]) || boolFact(facts["intersects_wetland"]) || boolFact(facts["intersects_nhd_area"]) {
		band, explanation = BandHigh, "The confirmed boundary intersects a mapped floodplain, wetland, or surface-water feature."
	} else if within(facts["nearest_wetland_distance_m"], 100) || positiveFact(facts["wetlands_within_100m_count"]) || positiveFact(facts["wetlands_within_500m_count"]) {
		band, explanation = BandModerate, "Mapped wetlands are present near the confirmed boundary."
	} else if !allFactsComplete(facts, "within_floodplain_polygon", "intersects_wetland", "intersects_nhd_area") {
		band, explanation = BandUnknown, "Water-receptor evidence is incomplete."
	}
	return category("WATER_RECEPTORS", "Water receptors", band, explanation, components, "DRAFT_GUIDANCE", "EPA draft guidance for reducing PFOA and PFOS risk in biosolids", epaDraftGuidanceURL)
}

func subsurfaceCategory(facts map[string]FactInput) VulnerabilityCategory {
	components := components(facts, "nearest_groundwater_well_depth_to_water_m", "soil_drainage_class", "soil_hydrologic_group", "soil_ponding_frequency_class", "soil_restrictive_layer_depth_cm", "soil_available_water_capacity")
	depth, ok := numericFact(facts["nearest_groundwater_well_depth_to_water_m"])
	band, explanation := BandUnknown, "No usable groundwater-depth indicator was available for comparison."
	if ok {
		switch {
		case depth <= 0.762:
			band, explanation = BandHigh, "The nearest measured groundwater-depth indicator is at or below Michigan's 30-inch site-separation benchmark; site-specific confirmation is still required."
		case depth <= 3:
			band, explanation = BandModerate, "The nearest measured groundwater-depth indicator is relatively shallow and merits comparison with the approved site record."
		default:
			band, explanation = BandLow, "The nearest measured groundwater-depth indicator is deeper than three meters; this is supporting evidence, not a site-specific groundwater determination."
		}
	}
	return category("SUBSURFACE_MOBILITY", "Subsurface mobility", band, explanation, components, "RULE_AND_GUIDANCE", "Michigan Part 24 management practices and EPA draft PFAS biosolids guidance", michiganBiosolidsURL)
}

func surfaceCategory(facts map[string]FactInput) VulnerabilityCategory {
	components := components(facts, "slope_degrees", "soil_erodibility_k_factor", "within_floodplain_polygon", "intersects_wetland", "intersects_nhd_area")
	maxSlope, slopeOK := rangeMaximum(facts["slope_degrees"])
	defaultSurfaceLimitDegrees := math.Atan(0.06) * 180 / math.Pi
	band, explanation := BandLow, "No direct mapped water overlap or default surface-slope concern was returned."
	if boolFact(facts["within_floodplain_polygon"]) || boolFact(facts["intersects_wetland"]) || boolFact(facts["intersects_nhd_area"]) || (slopeOK && maxSlope > defaultSurfaceLimitDegrees) {
		band, explanation = BandHigh, "The boundary has a mapped water/flood overlap or a sampled slope above Michigan's default six-percent surface-application limit."
	} else if !allFactsComplete(facts, "slope_degrees", "within_floodplain_polygon", "intersects_wetland", "intersects_nhd_area") {
		band, explanation = BandUnknown, "Surface-transport evidence is incomplete."
	}
	return category("SURFACE_TRANSPORT", "Surface transport", band, explanation, components, "RULE", "Michigan biosolids management practices", michiganBiosolidsURL)
}

func humanCategory(facts map[string]FactInput, cropOrUse string) VulnerabilityCategory {
	components := components(facts, "housing_units_within_1km", "housing_units_density_per_km2", "nearest_school_distance_m", "cdl_class", "is_cultivated", "dominant_crop_5y")
	crop := strings.ToLower(cropOrUse)
	highExposure := containsAny(crop, "leafy", "lettuce", "spinach", "root vegetable", "pasture", "grazing", "dairy", "animal feed", "poultry", "hens")
	lowerExposure := containsAny(crop, "grain", "fiber", "ethanol", "wheat", "corn for ethanol")
	band, explanation := BandUnknown, "The intended crop or use is missing, so human and food exposure cannot be compared completely."
	switch {
	case highExposure:
		band, explanation = BandHigh, "The supplied use matches an agricultural practice EPA's draft guidance identifies as having higher potential human exposure."
	case lowerExposure:
		band, explanation = BandLow, "The supplied use matches a lower-exposure crop example in EPA's draft guidance."
	case crop != "":
		band, explanation = BandModerate, "The supplied use is not one of EPA's named lower- or higher-exposure examples."
	case positiveFact(facts["housing_units_within_1km"]):
		band, explanation = BandModerate, "Nearby housing is present, but the intended field use remains unknown."
	}
	return category("HUMAN_FOOD_EXPOSURE", "Human and food exposure", band, explanation, components, "DRAFT_GUIDANCE", "EPA draft guidance for reducing PFOA and PFOS risk in biosolids", epaDraftGuidanceURL)
}

func uncertaintyCategory(input FieldInput) VulnerabilityCategory {
	components := []PlacementComponent{{FactName: "physical_evaluation", Label: "Physical evidence status", Value: mustJSON(input.PhysicalStatus), Source: "Stored field evaluation"}}
	band, explanation := BandLow, "Critical field facts are complete and adjacent evidence sources returned records."
	switch {
	case input.PhysicalEvaluationID == "" || input.PhysicalStatus == "":
		band, explanation = BandUnknown, "No physical-evidence record exists for this boundary."
	case input.PhysicalStatus != "SUCCEEDED" || input.PhysicalCriticalGaps > 0:
		band, explanation = BandHigh, "Critical physical evidence is incomplete or requires review."
	case input.PhysicalOtherGaps > 0 || !input.SupplementalAvailable:
		band, explanation = BandModerate, "Critical facts are complete, but one or more supporting sources are unavailable."
	}
	return category("DATA_UNCERTAINTY", "Evidence uncertainty", band, explanation, components, "PRODUCT_CONFIG", "PFAS Load Control evidence-completeness contract", "")
}

func category(key, label string, band Band, explanation string, components []PlacementComponent, authority, title, sourceURL string) VulnerabilityCategory {
	return VulnerabilityCategory{Key: key, Label: label, Band: band, Explanation: explanation, Components: components, AuthorityType: authority, SourceTitle: title, SourceURL: sourceURL, ConfigVersion: ConfigVersion}
}

func factMap(facts []FactInput) map[string]FactInput {
	result := make(map[string]FactInput, len(facts))
	for _, fact := range facts {
		result[fact.Name] = fact
	}
	return result
}
func components(facts map[string]FactInput, names ...string) []PlacementComponent {
	result := make([]PlacementComponent, 0, len(names))
	for _, name := range names {
		if fact, ok := facts[name]; ok {
			result = append(result, PlacementComponent{FactName: fact.Name, Label: fact.Label, State: fact.State, Value: fact.Value, Unit: fact.Unit, Source: fact.Source, SourceURL: fact.SourceURL})
		}
	}
	return result
}
func allFactsComplete(facts map[string]FactInput, names ...string) bool {
	for _, name := range names {
		if facts[name].State != "COMPLETE" {
			return false
		}
	}
	return true
}
func boolFact(fact FactInput) bool {
	var value bool
	return fact.State == "COMPLETE" && json.Unmarshal(fact.Value, &value) == nil && value
}
func positiveFact(fact FactInput) bool { value, ok := numericFact(fact); return ok && value > 0 }
func within(fact FactInput, limit float64) bool {
	value, ok := numericFact(fact)
	return ok && value <= limit
}
func numericFact(fact FactInput) (float64, bool) {
	if fact.State != "COMPLETE" {
		return 0, false
	}
	decoder := json.NewDecoder(bytes.NewReader(fact.Value))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return 0, false
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Float64()
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}
func rangeMaximum(fact FactInput) (float64, bool) {
	if fact.State != "COMPLETE" {
		return 0, false
	}
	var value struct {
		Max float64 `json:"max"`
	}
	if json.Unmarshal(fact.Value, &value) != nil {
		return 0, false
	}
	return value.Max, true
}
func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
func mustJSON(value any) json.RawMessage { encoded, _ := json.Marshal(value); return encoded }
func dispositionOrder(value Disposition) int {
	switch value {
	case DispositionEligible:
		return 0
	case DispositionReviewRequired:
		return 1
	default:
		return 2
	}
}
func roadDistance(value *float64) float64 {
	if value == nil {
		return math.MaxFloat64
	}
	return *value
}

func dryTons(wetMassKg, percentSolids string) (*big.Rat, error) {
	wet, err := positiveDecimal(wetMassKg)
	if err != nil {
		return nil, errors.New("wet mass must be a positive decimal")
	}
	solids, err := positiveDecimal(percentSolids)
	if err != nil || solids.Cmp(big.NewRat(100, 1)) > 0 {
		return nil, errors.New("total solids must be greater than zero and at most 100 percent")
	}
	result := new(big.Rat).Mul(wet, solids)
	result.Quo(result, big.NewRat(100, 1))
	result.Quo(result, big.NewRat(45359237, 50000))
	return result, nil
}
func positiveDecimal(value string) (*big.Rat, error) {
	parsed, err := decimal(value)
	if err != nil || parsed.Sign() <= 0 {
		return nil, errInvalidNumber
	}
	return parsed, nil
}
func nonnegativeDecimal(value string) (*big.Rat, error) {
	parsed, err := decimal(value)
	if err != nil || parsed.Sign() < 0 {
		return nil, errInvalidNumber
	}
	return parsed, nil
}
func decimal(value string) (*big.Rat, error) {
	parsed := new(big.Rat)
	if _, ok := parsed.SetString(strings.TrimSpace(value)); !ok {
		return nil, errInvalidNumber
	}
	return parsed, nil
}
func decimalOrZero(value string) (*big.Rat, error) {
	if value == "" {
		return new(big.Rat), nil
	}
	return decimal(value)
}
func minRat(left, right *big.Rat) *big.Rat {
	if left.Cmp(right) <= 0 {
		return new(big.Rat).Set(left)
	}
	return new(big.Rat).Set(right)
}
func formatRat(value *big.Rat) string {
	text := value.FloatString(6)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}
