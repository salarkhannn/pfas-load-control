package placement

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var (
	errSamplingLimitExceeded = errors.New("configured sampling target exceeds the request limit")
	errSamplingTargetUnmet   = errors.New("configured sampling target could not be placed inside the polygon")
)

type polygonSamplePlan struct {
	AlgorithmVersion     string
	Locations            [][]float64
	RequestedSampleCount int
	MaxRequestSamples    int
	MinimumSpacingMeters float64
	BoundaryNearSamples  int
	InteriorSamples      int
}

type sampleCandidate struct {
	point        []float64
	boundaryNear bool
}

// planPolygonScreeningSamples generates a reproducible screening plan for one valid
// single-ring polygon. It deliberately provides finite screening locations, not a claim
// of whole-field terrain coverage.
func planPolygonScreeningSamples(ring [][]float64, policy SamplingPolicy) (polygonSamplePlan, error) {
	plan := polygonSamplePlan{
		AlgorithmVersion:     policy.AlgorithmVersion,
		RequestedSampleCount: policy.TargetSampleCount,
		MaxRequestSamples:    policy.MaxRequestSamples,
		MinimumSpacingMeters: policy.MinimumSpacingMeters,
	}
	if policy.AlgorithmVersion != PolygonSamplingAlgorithmV1 || policy.TargetSampleCount < 3 || policy.MaxRequestSamples < 1 || policy.MinimumSpacingMeters <= 0 || policy.BoundaryNearDistanceMeters <= 0 {
		return plan, errors.New("sampling policy is incomplete or unsupported")
	}
	if policy.TargetSampleCount > policy.MaxRequestSamples {
		return plan, errSamplingLimitExceeded
	}
	if len(ring) < 4 {
		return plan, errSamplingTargetUnmet
	}

	anchor, err := interiorAnchor(ring)
	if err != nil {
		return plan, err
	}
	selected := []sampleCandidate{{point: anchor}}

	minX, maxX, minY, maxY := ringBounds(ring)
	quadrants := [][]float64{
		{minX + (maxX-minX)*0.25, minY + (maxY-minY)*0.25},
		{minX + (maxX-minX)*0.75, minY + (maxY-minY)*0.25},
		{minX + (maxX-minX)*0.25, minY + (maxY-minY)*0.75},
		{minX + (maxX-minX)*0.75, minY + (maxY-minY)*0.75},
	}
	validQuadrants := make([]sampleCandidate, 0, len(quadrants))
	for _, point := range quadrants {
		if !pointStrictlyInside(point, ring) {
			continue
		}
		validQuadrants = append(validQuadrants, sampleCandidate{point: point, boundaryNear: minimumDistanceToRingMeters(point, ring) <= policy.BoundaryNearDistanceMeters})
	}
	// Preserve the stable rectangle plan while ensuring irregular polygons get at least
	// two locations near different boundary segments before interior fill locations.
	for _, candidate := range validQuadrants {
		if candidate.boundaryNear {
			selected = appendCandidate(selected, candidate, policy.MinimumSpacingMeters, policy.TargetSampleCount)
		}
	}
	for edge := 0; boundaryNearCount(selected) < 2 && edge < len(ring)-1; edge++ {
		left, right := ring[edge], ring[edge+1]
		midpoint := []float64{(left[0] + right[0]) / 2, (left[1] + right[1]) / 2}
		for _, inset := range []float64{0.05, 0.1, 0.2, 0.35} {
			point := []float64{midpoint[0] + (anchor[0]-midpoint[0])*inset, midpoint[1] + (anchor[1]-midpoint[1])*inset}
			if pointStrictlyInside(point, ring) && minimumDistanceToRingMeters(point, ring) <= policy.BoundaryNearDistanceMeters {
				selected = appendCandidate(selected, sampleCandidate{point: point, boundaryNear: true}, policy.MinimumSpacingMeters, policy.TargetSampleCount)
				break
			}
		}
	}
	for _, candidate := range validQuadrants {
		selected = appendCandidate(selected, candidate, policy.MinimumSpacingMeters, policy.TargetSampleCount)
	}

	for divisions := 3; len(selected) < policy.TargetSampleCount && divisions <= 32; divisions++ {
		candidates := make([]sampleCandidate, 0, divisions*divisions)
		for row := 0; row < divisions; row++ {
			for column := 0; column < divisions; column++ {
				point := []float64{
					minX + (float64(column)+0.5)*(maxX-minX)/float64(divisions),
					minY + (float64(row)+0.5)*(maxY-minY)/float64(divisions),
				}
				if pointStrictlyInside(point, ring) {
					candidates = append(candidates, sampleCandidate{point: point, boundaryNear: minimumDistanceToRingMeters(point, ring) <= policy.BoundaryNearDistanceMeters})
				}
			}
		}
		// Farthest-first selection improves spatial spread while the coordinate tie-break
		// keeps generation byte-for-byte deterministic.
		sort.SliceStable(candidates, func(left, right int) bool {
			leftDistance := distanceToSelectedMeters(candidates[left].point, selected)
			rightDistance := distanceToSelectedMeters(candidates[right].point, selected)
			if math.Abs(leftDistance-rightDistance) > 1e-6 {
				return leftDistance > rightDistance
			}
			if candidates[left].point[1] != candidates[right].point[1] {
				return candidates[left].point[1] < candidates[right].point[1]
			}
			return candidates[left].point[0] < candidates[right].point[0]
		})
		for _, candidate := range candidates {
			selected = appendCandidate(selected, candidate, policy.MinimumSpacingMeters, policy.TargetSampleCount)
			if len(selected) == policy.TargetSampleCount {
				break
			}
		}
	}
	if len(selected) != policy.TargetSampleCount || boundaryNearCount(selected) < 2 {
		return plan, fmt.Errorf("%w: placed %d of %d locations with %d boundary-near", errSamplingTargetUnmet, len(selected), policy.TargetSampleCount, boundaryNearCount(selected))
	}

	plan.Locations = make([][]float64, 0, len(selected))
	for _, candidate := range selected {
		plan.Locations = append(plan.Locations, []float64{roundCoordinate(candidate.point[0]), roundCoordinate(candidate.point[1])})
		if candidate.boundaryNear {
			plan.BoundaryNearSamples++
		} else {
			plan.InteriorSamples++
		}
	}
	return plan, nil
}

func appendCandidate(selected []sampleCandidate, candidate sampleCandidate, minimumSpacingMeters float64, target int) []sampleCandidate {
	if len(selected) >= target || !finitePoint(candidate.point) {
		return selected
	}
	for _, existing := range selected {
		if coordinateDistanceMeters(existing.point, candidate.point) < minimumSpacingMeters {
			return selected
		}
	}
	return append(selected, candidate)
}

func interiorAnchor(ring [][]float64) ([]float64, error) {
	if centroid, ok := polygonCentroid(ring); ok && pointStrictlyInside(centroid, ring) {
		return centroid, nil
	}
	minX, maxX, minY, maxY := ringBounds(ring)
	var best []float64
	bestDistance := -1.0
	for divisions := 4; divisions <= 64; divisions *= 2 {
		for row := 0; row < divisions; row++ {
			for column := 0; column < divisions; column++ {
				point := []float64{minX + (float64(column)+0.5)*(maxX-minX)/float64(divisions), minY + (float64(row)+0.5)*(maxY-minY)/float64(divisions)}
				if !pointStrictlyInside(point, ring) {
					continue
				}
				distance := minimumDistanceToRingMeters(point, ring)
				if distance > bestDistance+1e-6 || (math.Abs(distance-bestDistance) <= 1e-6 && coordinateLess(point, best)) {
					best, bestDistance = point, distance
				}
			}
		}
		if best != nil {
			return best, nil
		}
	}
	return nil, errors.New("polygon has no deterministic interior anchor")
}

func polygonCentroid(ring [][]float64) ([]float64, bool) {
	originX, originY := ring[0][0], ring[0][1]
	doubleArea, x, y := 0.0, 0.0, 0.0
	for index := 0; index < len(ring)-1; index++ {
		leftX, leftY := ring[index][0]-originX, ring[index][1]-originY
		rightX, rightY := ring[index+1][0]-originX, ring[index+1][1]-originY
		cross := leftX*rightY - rightX*leftY
		doubleArea += cross
		x += (leftX + rightX) * cross
		y += (leftY + rightY) * cross
	}
	if math.Abs(doubleArea) <= 1e-15 {
		return nil, false
	}
	return []float64{originX + x/(3*doubleArea), originY + y/(3*doubleArea)}, true
}

func ringBounds(ring [][]float64) (float64, float64, float64, float64) {
	minX, maxX, minY, maxY := ring[0][0], ring[0][0], ring[0][1], ring[0][1]
	for _, point := range ring[:len(ring)-1] {
		minX, maxX = math.Min(minX, point[0]), math.Max(maxX, point[0])
		minY, maxY = math.Min(minY, point[1]), math.Max(maxY, point[1])
	}
	return minX, maxX, minY, maxY
}

func pointStrictlyInside(point []float64, ring [][]float64) bool {
	if !pointInPolygonOrBoundary(point[0], point[1], ring) {
		return false
	}
	for index := 0; index < len(ring)-1; index++ {
		if pointOnSegment(point, ring[index], ring[index+1]) {
			return false
		}
	}
	return true
}

func minimumDistanceToRingMeters(point []float64, ring [][]float64) float64 {
	minimum := math.Inf(1)
	for index := 0; index < len(ring)-1; index++ {
		minimum = math.Min(minimum, pointSegmentDistanceMeters(point, ring[index], ring[index+1]))
	}
	return minimum
}

func pointSegmentDistanceMeters(point, left, right []float64) float64 {
	latitude := (point[1] + left[1] + right[1]) / 3
	metersX := 111320.0 * math.Cos(latitude*math.Pi/180)
	const metersY = 110574.0
	px, py := point[0]*metersX, point[1]*metersY
	ax, ay := left[0]*metersX, left[1]*metersY
	bx, by := right[0]*metersX, right[1]*metersY
	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func coordinateDistanceMeters(left, right []float64) float64 {
	latitude := (left[1] + right[1]) / 2
	dx := (left[0] - right[0]) * 111320.0 * math.Cos(latitude*math.Pi/180)
	dy := (left[1] - right[1]) * 110574.0
	return math.Hypot(dx, dy)
}

func distanceToSelectedMeters(point []float64, selected []sampleCandidate) float64 {
	minimum := math.Inf(1)
	for _, candidate := range selected {
		minimum = math.Min(minimum, coordinateDistanceMeters(point, candidate.point))
	}
	return minimum
}

func boundaryNearCount(candidates []sampleCandidate) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.boundaryNear {
			count++
		}
	}
	return count
}

func finitePoint(point []float64) bool {
	return len(point) == 2 && !math.IsNaN(point[0]) && !math.IsNaN(point[1]) && !math.IsInf(point[0], 0) && !math.IsInf(point[1], 0)
}

func coordinateLess(left, right []float64) bool {
	if right == nil {
		return true
	}
	if left[1] != right[1] {
		return left[1] < right[1]
	}
	return left[0] < right[0]
}

func roundCoordinate(value float64) float64 {
	return math.Round(value*1e9) / 1e9
}
