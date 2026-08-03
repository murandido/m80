package ticket

import "testing"

func TestCleanCabin(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		cabin string
		want  string
		why   string
	}{
		{
			name:  "the broken escape the vendor actually sends",
			cabin: "Econu00f4mica",
			want:  "Econômica",
			why:   "this exact string reached a customer's PDF - it is a \\u00f4 that lost its backslash inside Traveos",
		},
		{
			name:  "already correct portuguese",
			cabin: "Econômica",
			want:  "Econômica",
		},
		{
			name:  "english is translated",
			cabin: "Economy",
			want:  "Econômica",
		},
		{
			name:  "case and spacing do not matter",
			cabin: "  PREMIUM   ECONOMY ",
			want:  "Econômica Premium",
		},
		{
			name:  "business",
			cabin: "Business",
			want:  "Executiva",
		},
		{
			name:  "first",
			cabin: "First Class",
			want:  "Primeira",
		},
		{
			name:  "a cabin nobody has seen survives untouched",
			cabin: "Mundo Plus",
			want:  "Mundo Plus",
			why:   "the dictionary is a courtesy, not a filter: an unknown class must reach the passenger the way the airline wrote it",
		},
		{
			name:  "empty stays empty",
			cabin: "   ",
			want:  "",
		},
		{
			name:  "a word that merely contains a u is not an escape",
			cabin: "Aubade",
			want:  "Aubade",
			why:   "u+bade is four hex characters; without the digit guard this would decode into a stray rune",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := cleanCabin(testCase.cabin); got != testCase.want {
				t.Errorf("cleanCabin(%q) = %q, want %q\n%s",
					testCase.cabin, got, testCase.want, testCase.why)
			}
		})
	}
}
