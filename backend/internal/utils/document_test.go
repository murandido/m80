package utils

import "testing"

func TestNormalizeCPF(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"masked", "529.982.247-25", "52998224725", true},
		{"bare", "52998224725", "52998224725", true},
		{"surrounded by noise", " CPF 529 982 247 25 ", "52998224725", true},
		{"wrong check digit", "52998224726", "", false},
		{"too short", "5299822472", "", false},
		{"too long", "529982247251", "", false},
		{"no digits", "sem documento", "", false},
		// The two placeholders that actually show up: Traveos returns the first when the
		// airline has no document on file. Both satisfy the check digits, so only an
		// explicit rejection keeps them from becoming a shared beneficiary.
		{"zeros placeholder", "000.000.000-00", "", false},
		{"repeated ones placeholder", "111.111.111-11", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeCPF(tc.input)
			if ok != tc.ok {
				t.Fatalf("NormalizeCPF(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("NormalizeCPF(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMaskCPF(t *testing.T) {
	if got, want := MaskCPF("52998224725"), "***.982.247-**"; got != want {
		t.Errorf("MaskCPF = %q, want %q", got, want)
	}
	// The first three digits and the check digits never reach the screen, so two people
	// differing only there look alike - acceptable, and better than storing the number.
	if got := MaskCPF("529982247"); got != "" {
		t.Errorf("MaskCPF of a non-normalized value = %q, want empty", got)
	}
}

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"João da Silva", "JOAO DA SILVA"},
		{"  joão   DA  silva  ", "JOAO DA SILVA"},
		{"ÁGATA MÜLLER-ANDRÉ", "AGATA MULLER-ANDRE"},
		{"", ""},
	}

	for _, tc := range cases {
		if got := NormalizeName(tc.input); got != tc.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// The whole point of the fallback key is that these two spellings are one person.
func TestNormalizeNameCollapsesSpellings(t *testing.T) {
	if NormalizeName("joão  da silva") != NormalizeName("JOAO DA SILVA") {
		t.Error("accent and spacing variants of the same name did not collapse")
	}
}
