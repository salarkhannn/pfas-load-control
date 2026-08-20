package coordination

import (
	"errors"
	"testing"
)

func TestNextWorkflowStatusRequiresOrderedRoles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, current, role, want string
	}{
		{name: "farmer begins", current: "NOT_STARTED", role: "FARMER", want: "FARMER_CONFIRMED"},
		{name: "contractor follows farmer", current: "FARMER_CONFIRMED", role: "CONTRACTOR", want: "CONTRACTOR_CONFIRMED"},
		{name: "plant completes coordination", current: "CONTRACTOR_CONFIRMED", role: "PLANT", want: "PLANT_CONFIRMED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nextWorkflowStatus(tc.current, tc.role)
			if err != nil || got != tc.want {
				t.Fatalf("nextWorkflowStatus() = %q, %v; want %q, nil", got, err, tc.want)
			}
		})
	}
}

func TestNextWorkflowStatusRejectsUnsafeTransitions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ current, role string }{
		{current: "NOT_STARTED", role: "CONTRACTOR"},
		{current: "NOT_STARTED", role: "PLANT"},
		{current: "FARMER_CONFIRMED", role: "PLANT"},
		{current: "PLANT_CONFIRMED", role: "PLANT"},
		{current: "REJECTED", role: "FARMER"},
	} {
		if _, err := nextWorkflowStatus(tc.current, tc.role); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("nextWorkflowStatus(%q, %q) error = %v; want ErrInvalidTransition", tc.current, tc.role, err)
		}
	}
}
