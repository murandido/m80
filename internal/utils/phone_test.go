package utils

import "testing"

func TestClearPhone(t *testing.T) {
	// test table
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "normal",
			input:    "5511999999999",
			expected: "5511999999999",
		},
		{
			name:     "with letters",
			input:    "551sdf1999f999f999a",
			expected: "5511999999999",
		},
		{
			name:     "with letters and symbols",
			input:    "+551sdf1999f999f999++-a",
			expected: "5511999999999",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ClearPhone(test.input)

			if test.expected != result {
				t.Errorf("expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestValidateBRPhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty",
			input:    "",
			expected: false,
		},
		{
			name:     "normal",
			input:    "11999999999",
			expected: true,
		},
		{
			name:     "with symbols",
			input:    "(11) 99999-9999",
			expected: true,
		},
		{
			name:     "landline phone",
			input:    "11 2345-6789",
			expected: true,
		},
		{
			name:     "few digits",
			input:    "123456",
			expected: false,
		},
		{
			name:     "with DDI",
			input:    "+5511999999999",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateBRPhone(test.input)

			if test.expected != result {
				t.Errorf("expected %t, got %t", test.expected, result)
			}
		})
	}
}
