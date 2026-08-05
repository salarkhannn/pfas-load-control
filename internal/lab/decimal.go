package lab

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^([+-]?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$`)

func canonicalDecimal(input string) (string, error) {
	value := strings.ReplaceAll(strings.TrimSpace(input), ",", "")
	matches := decimalPattern.FindStringSubmatch(value)
	if matches == nil {
		return "", fmt.Errorf("%q is not a decimal number", input)
	}
	if matches[1] == "-" {
		return "", errors.New("value must not be negative")
	}

	digits := matches[2] + matches[3]
	if strings.Trim(digits, "0") == "" {
		return "0", nil
	}
	scale := len(matches[3])
	if matches[4] != "" {
		var exponent int
		if _, err := fmt.Sscanf(matches[4], "%d", &exponent); err != nil {
			return "", fmt.Errorf("invalid decimal exponent: %w", err)
		}
		scale -= exponent
	}

	if scale <= 0 {
		return strings.TrimLeft(digits+strings.Repeat("0", -scale), "0"), nil
	}
	if scale >= len(digits) {
		fraction := strings.TrimRight(strings.Repeat("0", scale-len(digits))+digits, "0")
		return "0." + fraction, nil
	}
	integer := strings.TrimLeft(digits[:len(digits)-scale], "0")
	if integer == "" {
		integer = "0"
	}
	fraction := strings.TrimRight(digits[len(digits)-scale:], "0")
	if fraction == "" {
		return integer, nil
	}
	return integer + "." + fraction, nil
}

func decimalRat(input string) (*big.Rat, error) {
	canonical, err := canonicalDecimal(input)
	if err != nil {
		return nil, err
	}
	value, ok := new(big.Rat).SetString(canonical)
	if !ok {
		return nil, fmt.Errorf("cannot represent %q as an exact decimal", input)
	}
	return value, nil
}

func normalizeConcentration(value, unit, basis string, percentSolids *string) (*string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	amount, err := decimalRat(value)
	if err != nil {
		return nil, err
	}

	switch unit {
	case "UG_KG", "NG_G":
	case "MG_KG":
		amount.Mul(amount, big.NewRat(1000, 1))
	case "UG_L":
		return nil, nil
	default:
		return nil, nil
	}

	switch basis {
	case "DRY":
	case "WET":
		if percentSolids == nil || strings.TrimSpace(*percentSolids) == "" {
			return nil, nil
		}
		solids, err := decimalRat(*percentSolids)
		if err != nil {
			return nil, fmt.Errorf("invalid percent solids: %w", err)
		}
		if solids.Sign() <= 0 || solids.Cmp(big.NewRat(100, 1)) > 0 {
			return nil, errors.New("percent solids must be greater than 0 and at most 100")
		}
		amount.Mul(amount, big.NewRat(100, 1))
		amount.Quo(amount, solids)
	default:
		return nil, nil
	}

	normalized := strings.TrimRight(strings.TrimRight(amount.FloatString(12), "0"), ".")
	if normalized == "" {
		normalized = "0"
	}
	return &normalized, nil
}

func decimalPointer(input *string) (*string, error) {
	if input == nil || strings.TrimSpace(*input) == "" {
		return nil, nil
	}
	value, err := canonicalDecimal(*input)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
