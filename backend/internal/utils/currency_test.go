package utils

import (
	"errors"
	"testing"
)

func TestParseBRLCents(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"whole number", "30", 3000},
		{"comma decimal", "16,5", 1650},
		{"period decimal", "16.5", 1650},
		{"comma cents", "189,22", 18922},
		{"period cents", "189.22", 18922},
		{"brazilian thousands", "1.500,32", 150032},
		{"us thousands", "1,500.32", 150032},
		{"millions", "1.234.567,89", 123456789},
		{"leading zero", "0,05", 5},
		{"no integer part", ",5", 50},
		{"surrounding spaces", "  340,36  ", 34036},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBRLCents(tt.input)
			if err != nil {
				t.Fatalf("ParseBRLCents(%q) returned %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseBRLCents(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// The whole point of accepting both separators is that the same amount typed
// either way must land on the same integer - anything else silently persists a
// different price depending on who typed it.
func TestParseBRLCents_CommaAndPeriodAreEquivalent(t *testing.T) {
	pairs := [][2]string{
		{"16,5", "16.5"},
		{"189,22", "189.22"},
		{"1.500,32", "1,500.32"},
		{"3037,72", "3037.72"},
	}

	for _, pair := range pairs {
		withComma, err := ParseBRLCents(pair[0])
		if err != nil {
			t.Fatalf("ParseBRLCents(%q) returned %v", pair[0], err)
		}
		withPeriod, err := ParseBRLCents(pair[1])
		if err != nil {
			t.Fatalf("ParseBRLCents(%q) returned %v", pair[1], err)
		}
		if withComma != withPeriod {
			t.Fatalf("%q parsed as %d but %q parsed as %d", pair[0], withComma, pair[1], withPeriod)
		}
	}
}

func TestParseBRLCents_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"three decimals", "16,555", ErrTooManyDecimals},
		{"empty", "", ErrInvalidDecimal},
		{"only separator", ",", ErrInvalidDecimal},
		{"letters", "16a", ErrInvalidDecimal},
		{"k suffix is not currency", "31k", ErrInvalidDecimal},
		{"negative", "-16,5", ErrInvalidDecimal},
		{"currency symbol", "R$ 16,50", ErrInvalidDecimal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBRLCents(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseBRLCents(%q) err = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// scale 3 is how a "31k" point count is read once the suffix is stripped, and
// scale 0 how a bare integer is.
func TestParseScaledDecimal_OtherScales(t *testing.T) {
	tests := []struct {
		input    string
		decimals int
		want     int
		wantErr  error
	}{
		{"31", 3, 31000, nil},
		{"360", 3, 360000, nil},
		{"89,912", 3, 89912, nil},
		{"90,8", 3, 90800, nil},
		{"90.8", 3, 90800, nil},
		{"89,9125", 3, 0, ErrTooManyDecimals},
		{"31000", 0, 31000, nil},
		{"31,5", 0, 0, ErrTooManyDecimals},
	}

	for _, tt := range tests {
		got, err := ParseScaledDecimal(tt.input, tt.decimals)
		if tt.wantErr != nil {
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseScaledDecimal(%q, %d) err = %v, want %v", tt.input, tt.decimals, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseScaledDecimal(%q, %d) returned %v", tt.input, tt.decimals, err)
		}
		if got != tt.want {
			t.Fatalf("ParseScaledDecimal(%q, %d) = %d, want %d", tt.input, tt.decimals, got, tt.want)
		}
	}
}
