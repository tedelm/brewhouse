package service

import (
	"strings"
)

// ConvertQty converts qty from one unit to another within mass or volume families.
// Compatible pairs: g/kg and ml/L (case-insensitive). Empty from/to treated as equal.
// Incompatible units fall back to a 1:1 numeric compare (no conversion).
func ConvertQty(qty float64, from, to string) (float64, error) {
	from = normalizeUnit(from)
	to = normalizeUnit(to)
	if from == "" || to == "" || from == to {
		return qty, nil
	}
	fromBase, fromFam, okFrom := toBaseUnit(from)
	toBase, toFam, okTo := toBaseUnit(to)
	if !okFrom || !okTo || fromFam != toFam {
		return qty, nil
	}
	baseQty := qty * fromBase
	return baseQty / toBase, nil
}

func normalizeUnit(u string) string {
	u = strings.TrimSpace(strings.ToLower(u))
	switch u {
	case "l":
		return "L"
	case "ml":
		return "ml"
	case "kg":
		return "kg"
	case "g":
		return "g"
	default:
		return u
	}
}

// toBaseUnit returns multiplier to base (g or ml) and family name.
func toBaseUnit(u string) (mult float64, family string, ok bool) {
	switch normalizeUnit(u) {
	case "g":
		return 1, "mass", true
	case "kg":
		return 1000, "mass", true
	case "ml":
		return 1, "volume", true
	case "L":
		return 1000, "volume", true
	default:
		return 0, "", false
	}
}

func defaultUnit(u string) string {
	u = normalizeUnit(u)
	if u == "" {
		return "kg"
	}
	return u
}
