package responseplan

import (
	"testing"
)

func TestLandfillSourceFiltersToActiveAcceptingTypeII(t *testing.T) {
	t.Parallel()
	body := []byte(`{"type":"FeatureCollection","features":[
          {"properties":{"wdsid":1,"specificsitename":"Valid","actcode":"II","facilitytype":"Type II MSW Landfill","addrline1":"1 Main St","city":"Town","countyname":"County","latdeccord":"42","longdeccord":"-84","disposalareastatus":"Active - Accepting","landfilllink":"https://example.com/1"}},
          {"properties":{"wdsid":2,"specificsitename":"Wrong type","actcode":"III-IND","facilitytype":"Type III","addrline1":"2 Main St","city":"Town","countyname":"County","latdeccord":"42.1","longdeccord":"-84.1","disposalareastatus":"Active - Accepting","landfilllink":"https://example.com/2"}},
          {"properties":{"wdsid":3,"specificsitename":"Not accepting","actcode":"II","facilitytype":"Type II MSW Landfill","addrline1":"3 Main St","city":"Town","countyname":"County","latdeccord":"42.2","longdeccord":"-84.2","disposalareastatus":"Inactive","landfilllink":"https://example.com/3"}}
		]}`)
	facilities, err := parseLandfills(body, 42, -84)
	if err != nil {
		t.Fatalf("parseLandfills() error = %v", err)
	}
	if len(facilities) != 1 {
		t.Fatalf("parseLandfills() returned %d facilities, want 1", len(facilities))
	}
	if facilities[0].WDSID != "1" || facilities[0].Name != "Valid" || facilities[0].DisposalAreaStatus != "Active - Accepting" {
		t.Fatalf("parseLandfills() returned %#v", facilities[0])
	}
}

func TestHaversineDistanceIsZeroForSamePoint(t *testing.T) {
	t.Parallel()
	if distance := haversineKM(42, -84, 42, -84); distance != 0 {
		t.Fatalf("haversineKM() = %f", distance)
	}
}
