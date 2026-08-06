package evidence

import (
	"reflect"
	"testing"
)

func TestSplitCodesKeepsOnlyNAICSValues(t *testing.T) {
	t.Parallel()

	got := splitCodes("332999, 332996, NA;32512")
	want := []string{"332999", "332996", "32512"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitCodes() = %#v, want %#v", got, want)
	}
}
