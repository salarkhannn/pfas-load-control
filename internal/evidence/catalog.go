package evidence

const (
	FieldSetVersion    = "pfas-physical-v1"
	AggregationVersion = "anchor-grid-2x2-v1"
)

type aggregateMethod string

const (
	aggregateAnyTrue      aggregateMethod = "ANY_TRUE"
	aggregateMinimum      aggregateMethod = "MINIMUM"
	aggregateMaximum      aggregateMethod = "MAXIMUM"
	aggregateNumericRange aggregateMethod = "MIN_MAX_MEDIAN"
	aggregateDistribution aggregateMethod = "DISTRIBUTION"
)

type fieldSpec struct {
	Name     string
	Label    string
	Category string
	Type     string
	Method   aggregateMethod
	Critical bool
}

var physicalFieldSpecs = []fieldSpec{
	{Name: "within_floodplain_polygon", Label: "Floodplain intersects field", Category: "WATER", Type: "bool", Method: aggregateAnyTrue, Critical: true},
	{Name: "intersects_wetland", Label: "Wetland intersects field", Category: "WATER", Type: "bool", Method: aggregateAnyTrue, Critical: true},
	{Name: "nearest_wetland_distance_m", Label: "Nearest wetland", Category: "WATER", Type: "float", Method: aggregateMinimum, Critical: true},
	{Name: "wetlands_within_100m_count", Label: "Wetlands within 100 m", Category: "WATER", Type: "int", Method: aggregateMaximum},
	{Name: "wetlands_within_500m_count", Label: "Wetlands within 500 m", Category: "WATER", Type: "int", Method: aggregateMaximum},
	{Name: "intersects_nhd_area", Label: "Mapped surface water intersects field", Category: "WATER", Type: "bool", Method: aggregateAnyTrue, Critical: true},
	{Name: "nearest_flowline_name", Label: "Nearby mapped flowlines", Category: "WATER", Type: "string", Method: aggregateDistribution},
	{Name: "nearest_waterbody_name", Label: "Nearby waterbodies", Category: "WATER", Type: "string", Method: aggregateDistribution},
	{Name: "huc_12_name", Label: "Watershed", Category: "WATER", Type: "string", Method: aggregateDistribution},
	{Name: "surface_water_permanence_pct", Label: "Surface-water permanence", Category: "WATER", Type: "float", Method: aggregateNumericRange},
	{Name: "nearest_groundwater_well_depth_to_water_m", Label: "Nearest measured depth to groundwater", Category: "WATER", Type: "float", Method: aggregateMinimum, Critical: true},
	{Name: "within_water_service_area", Label: "Public water service covers field", Category: "WATER", Type: "bool", Method: aggregateAnyTrue},
	{Name: "water_system_name", Label: "Water systems", Category: "WATER", Type: "string", Method: aggregateDistribution},
	{Name: "slope_degrees", Label: "Slope", Category: "SOIL", Type: "float", Method: aggregateNumericRange, Critical: true},
	{Name: "soil_drainage_class", Label: "Soil drainage", Category: "SOIL", Type: "string", Method: aggregateDistribution, Critical: true},
	{Name: "soil_hydrologic_group", Label: "Hydrologic soil group", Category: "SOIL", Type: "string", Method: aggregateDistribution, Critical: true},
	{Name: "soil_ponding_frequency_class", Label: "Ponding frequency", Category: "SOIL", Type: "string", Method: aggregateDistribution, Critical: true},
	{Name: "soil_restrictive_layer_depth_cm", Label: "Shallowest restrictive soil layer", Category: "SOIL", Type: "float", Method: aggregateMinimum, Critical: true},
	{Name: "soil_restrictive_layer_kind", Label: "Restrictive soil layers", Category: "SOIL", Type: "string", Method: aggregateDistribution},
	{Name: "soil_available_water_capacity", Label: "Available soil water capacity", Category: "SOIL", Type: "float", Method: aggregateNumericRange},
	{Name: "soil_erodibility_k_factor", Label: "Soil erodibility", Category: "SOIL", Type: "float", Method: aggregateNumericRange},
	{Name: "housing_units_within_1km", Label: "Homes within 1 km", Category: "PEOPLE", Type: "int", Method: aggregateMaximum, Critical: true},
	{Name: "housing_units_density_per_km2", Label: "Housing density", Category: "PEOPLE", Type: "float", Method: aggregateMaximum, Critical: true},
	{Name: "nearest_school_distance_m", Label: "Nearest school", Category: "PEOPLE", Type: "float", Method: aggregateMinimum, Critical: true},
	{Name: "cdl_class", Label: "Mapped land cover", Category: "LAND", Type: "string", Method: aggregateDistribution},
	{Name: "is_cultivated", Label: "Cultivated land present", Category: "LAND", Type: "bool", Method: aggregateAnyTrue},
	{Name: "dominant_crop_5y", Label: "Dominant crops", Category: "LAND", Type: "string", Method: aggregateDistribution},
	{Name: "nearest_road_distance_m", Label: "Nearest road", Category: "ACCESS", Type: "float", Method: aggregateMinimum},
	{Name: "nearest_road_class", Label: "Nearby road classes", Category: "ACCESS", Type: "string", Method: aggregateDistribution},
	{Name: "nearest_road_surface", Label: "Nearby road surfaces", Category: "ACCESS", Type: "string", Method: aggregateDistribution},
}

func physicalFieldNames() []string {
	result := make([]string, len(physicalFieldSpecs))
	for index, spec := range physicalFieldSpecs {
		result[index] = spec.Name
	}
	return result
}

func specByName(name string) (fieldSpec, bool) {
	for _, spec := range physicalFieldSpecs {
		if spec.Name == name {
			return spec, true
		}
	}
	return fieldSpec{}, false
}
