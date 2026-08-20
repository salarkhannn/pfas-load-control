package responseplan

import (
	"errors"
	"testing"

	"github.com/salarkhannn/pfas-load-control/internal/evidence"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
)

func TestCanonicalFacilityState(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"Michigan":   "MI",
		" michigan ": "MI",
		"MI":         "MI",
		"Ohio":       "Ohio",
	} {
		if got := canonicalFacilityState(input); got != want {
			t.Errorf("canonicalFacilityState(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExcludeTreatmentPlant(t *testing.T) {
	t.Parallel()

	facilities := []evidence.ECHOFacility{
		{RegistryID: "self", Name: "BLISSFIELD WWTP"},
		{RegistryID: "lead", Name: "Crescent Manufacturing"},
	}
	got := excludeTreatmentPlant(facilities, "Blissfield Wastewater Treatment Plant")
	if len(got) != 1 || got[0].RegistryID != "lead" {
		t.Fatalf("excludeTreatmentPlant() = %#v", got)
	}
}

func TestElevatedResponseTasksEnforceRateCap(t *testing.T) {
	t.Parallel()
	tasks := taskDefinitions("ELEVATED")
	assertTask(t, tasks, "ENFORCE_RATE_CAP", "ENFORCED")
	assertTask(t, tasks, "SOURCE_EFFLUENT_SAMPLE", "REQUIRED")
	assertNoTask(t, tasks, "BLOCK_LAND_APPLICATION")
	assertNoTask(t, tasks, "ARRANGE_ALTERNATIVE_MANAGEMENT")
}

func TestProhibitedResponseTasksBlockLandApplicationAndPrepareAlternatives(t *testing.T) {
	t.Parallel()
	tasks := taskDefinitions("PROHIBITED")
	assertTask(t, tasks, "BLOCK_LAND_APPLICATION", "ENFORCED")
	assertTask(t, tasks, "NOTIFY_EGLE", "REQUIRED")
	assertTask(t, tasks, "ARRANGE_ALTERNATIVE_MANAGEMENT", "DRAFT")
	assertNoTask(t, tasks, "ENFORCE_RATE_CAP")
}

func TestRouteCandidatesPreferUsableRoutesAndPreserveFailures(t *testing.T) {
	t.Parallel()
	dropped := "not found"
	unreachable := "unreachable_or_snapped"
	duration := 25.0
	facilities := []Landfill{
		{WDSID: "a", StraightlineKM: 5},
		{WDSID: "b", StraightlineKM: 10},
		{WDSID: "c", StraightlineKM: 15},
	}
	routes := mireye.DistanceResult{
		ResolvedDestinations: []mireye.ResolvedPoint{{}, {}, {Error: &dropped}},
		Legs: []mireye.DistanceLeg{
			{DestinationIndex: 0, Flag: &unreachable},
			{DestinationIndex: 1, DistanceKM: 20, DurationMinutes: &duration},
		},
	}
	result := routeCandidates(facilities, routes, nil)
	if result[0].WDSID != "b" || result[0].RouteStatus != "ROUTED" {
		t.Fatalf("first candidate = %#v, want routed facility b", result[0])
	}
	statuses := map[string]string{}
	for _, candidate := range result {
		statuses[candidate.WDSID] = candidate.RouteStatus
	}
	if statuses["a"] != "UNREACHABLE" || statuses["c"] != "DROPPED" {
		t.Fatalf("statuses = %#v", statuses)
	}

	withoutRoutes := routeCandidates(facilities, mireye.DistanceResult{}, errors.New("offline"))
	for _, candidate := range withoutRoutes {
		if candidate.RouteStatus != "NOT_ROUTED" {
			t.Fatalf("route failure status = %q, want NOT_ROUTED", candidate.RouteStatus)
		}
	}
}

func assertTask(t *testing.T, tasks []ResponseTask, code, state string) {
	t.Helper()
	for _, task := range tasks {
		if task.Code == code {
			if task.State != state {
				t.Fatalf("task %s state = %s, want %s", code, task.State, state)
			}
			return
		}
	}
	t.Fatalf("task %s not found", code)
}

func assertNoTask(t *testing.T, tasks []ResponseTask, code string) {
	t.Helper()
	for _, task := range tasks {
		if task.Code == code {
			t.Fatalf("unexpected task %s", code)
		}
	}
}
