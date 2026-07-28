package utils

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidDecimal is returned for anything that isn't a plain positive number
// written with the separators below.
var ErrInvalidDecimal = errors.New("invalid decimal number")

// ErrTooManyDecimals is returned when a value carries more precision than the
// target unit can hold - e.g. "16,555" as a currency amount, or "89,9125k" as a
// point count. Rounding it silently would persist a number the operator never
// typed, so it is refused instead.
var ErrTooManyDecimals = errors.New("too many decimal places")

// ParseBRLCents parses a monetary amount written the way operators actually type
// it into integer cents - the unit every money column in this project stores.
//
//	"30"        -> 3000
//	"16,5"      -> 1650
//	"16.5"      -> 1650
//	"189,22"    -> 18922
//	"1.500,32"  -> 150032
//	"1,500.32"  -> 150032
//	"16,555"    -> ErrTooManyDecimals
func ParseBRLCents(value string) (int, error) {
	return ParseScaledDecimal(value, 2)
}

// ParseScaledDecimal parses a positive decimal number and returns it scaled by
// 10^decimals, so the result is an exact integer in the target unit (cents for
// money, single points for a "31k" point count).
//
// Comma and period are both accepted as separators, because both show up in the
// same WhatsApp group - "16,5" and "16.5" are the same number. The rule for
// telling them apart is positional rather than locale-based: with two or more
// separators present the last one is the decimal point and the earlier ones are
// thousands groups ("1.500,32" and "1,500.32" both mean 1500.32); with a single
// separator it is read as the decimal point.
//
// That last case is where a thousands group written alone would be misread -
// but only when it is short enough to pass for a fraction: "1.500" as money
// carries three digits where cents hold two and is refused outright, while
// "1.50" is legitimately one and a half. Callers that can cross-check the parsed
// value should still do so; the EMISSAO parser validates it against the total
// the message itself carries.
func ParseScaledDecimal(value string, decimals int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("%w: empty", ErrInvalidDecimal)
	}

	// The last separator is the decimal point; every earlier one is a thousands
	// group and carries no meaning, so it is simply dropped.
	integerPart, fractionPart := trimmed, ""
	if cut := strings.LastIndexAny(trimmed, ",."); cut >= 0 {
		integerPart, fractionPart = trimmed[:cut], trimmed[cut+1:]
	}
	integerPart = strings.NewReplacer(",", "", ".", "").Replace(integerPart)

	if !isDigits(integerPart) || !isDigits(fractionPart) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDecimal, value)
	}
	if integerPart == "" && fractionPart == "" {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDecimal, value)
	}
	if len(fractionPart) > decimals {
		return 0, fmt.Errorf("%w: %q has more than %d", ErrTooManyDecimals, value, decimals)
	}

	// Right-pad so the fraction always lands on the target unit: with 2 decimals
	// "16,5" means 16.50, not 16.05.
	fractionPart += strings.Repeat("0", decimals-len(fractionPart))

	units, err := strconv.Atoi(integerPart + fractionPart)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDecimal, value)
	}
	return units, nil
}

// isDigits reports whether s is made up exclusively of ASCII digits. An empty
// string counts as digits - "0,5" has an empty integer part, ",5" style input
// has none at all, and both are legitimate.
func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
