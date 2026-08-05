package lab

import "testing"

func TestCanonicalDecimal(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"0":         "0",
		"00.000":    "0",
		"20.00":     "20",
		"0.00100":   "0.001",
		"1,250.50":  "1250.5",
		"1.20e3":    "1200",
		"1.20e-3":   "0.0012",
		"00012.340": "12.34",
	}
	for input, expected := range tests {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			actual, err := canonicalDecimal(input)
			if err != nil {
				t.Fatalf("canonicalDecimal(%q): %v", input, err)
			}
			if actual != expected {
				t.Fatalf("canonicalDecimal(%q) = %q; want %q", input, actual, expected)
			}
		})
	}
}

func TestNormalizeConcentration(t *testing.T) {
	t.Parallel()
	percentSolids := "25"
	tests := []struct {
		name     string
		value    string
		unit     string
		basis    string
		solids   *string
		expected string
	}{
		{name: "micrograms per kilogram dry", value: "12.5", unit: "UG_KG", basis: "DRY", expected: "12.5"},
		{name: "nanograms per gram dry", value: "12.5", unit: "NG_G", basis: "DRY", expected: "12.5"},
		{name: "milligrams per kilogram dry", value: "0.02", unit: "MG_KG", basis: "DRY", expected: "20"},
		{name: "wet basis", value: "5", unit: "UG_KG", basis: "WET", solids: &percentSolids, expected: "20"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			actual, err := normalizeConcentration(test.value, test.unit, test.basis, test.solids)
			if err != nil {
				t.Fatal(err)
			}
			if actual == nil || *actual != test.expected {
				t.Fatalf("normalized value = %v; want %s", actual, test.expected)
			}
		})
	}
}
