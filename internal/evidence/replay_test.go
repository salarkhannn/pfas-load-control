package evidence

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAggregateFrozenFactUsesLiveWholeFieldRule(t *testing.T) {
	values := []json.RawMessage{json.RawMessage("false"), json.RawMessage("false"), json.RawMessage("true"), json.RawMessage("false"), json.RawMessage("false")}
	fact, err := AggregateFrozenFact("intersects_wetland", values, "Mireye frozen response", "https://api.mireye.com/v1/physical/fields", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if string(fact.Value) != "true" || fact.OKCount != 5 || fact.State != "COMPLETE" || !fact.Critical {
		t.Fatalf("unexpected aggregate: %#v", fact)
	}
}
