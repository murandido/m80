package supplierreport

import (
	"strings"
	"testing"
)

// A payment figure without thousands separators is a figure waiting to be misread by a
// factor of ten, which is the whole reason this does not reuse the bot's formatCents.
func TestFormatBRL(t *testing.T) {
	for _, tc := range []struct {
		cents int
		want  string
	}{
		{0, "R$ 0,00"},
		{5, "R$ 0,05"},
		{99, "R$ 0,99"},
		{1650, "R$ 16,50"},
		{100000, "R$ 1.000,00"},
		{2000000, "R$ 20.000,00"},
		{123456789, "R$ 1.234.567,89"},
		{-82500, "-R$ 825,00"},
	} {
		if got := FormatBRL(tc.cents); got != tc.want {
			t.Errorf("FormatBRL(%d): got %q, want %q", tc.cents, got, tc.want)
		}
	}
}

func TestFormatPoints(t *testing.T) {
	for _, tc := range []struct {
		points int
		want   string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.000"},
		{50000, "50.000"},
		{1500000, "1.500.000"},
		{-20000, "-20.000"},
	} {
		if got := FormatPoints(tc.points); got != tc.want {
			t.Errorf("FormatPoints(%d): got %q, want %q", tc.points, got, tc.want)
		}
	}
}

// Program first, airline second, and the order is not interchangeable: Smiles is its own
// brand and would be wrong drawn as GOL, while TudoAzul is correctly drawn as AZUL.
func TestLogoFor_PrefersTheProgramOverItsAirline(t *testing.T) {
	gol := "GOL"

	smiles := string(LogoFor("SMILES", &gol))
	if !strings.Contains(smiles, "#ff7020") {
		t.Error("SMILES must resolve to its own artwork, not GOL's")
	}

	// A program with no artwork of its own falls through to the airline's.
	tudoAzul := "AZUL"
	if LogoFor("TUDOAZUL", &tudoAzul) == "" {
		t.Error("a program without its own logo must fall back to its default airline's")
	}

	// Neither known, and the caller's template prints the program name instead.
	unknown := "NOPE"
	if got := LogoFor("ALSONOTHING", &unknown); got != "" {
		t.Errorf("an unknown brand must resolve to nothing, got %q", got)
	}
	if got := LogoFor("ALSONOTHING", nil); got != "" {
		t.Errorf("a nil airline must not panic and must resolve to nothing, got %q", got)
	}
}

// Every logo the report can draw has to carry a viewBox, or WeasyPrint renders it at its
// intrinsic size and the header layout breaks. SMILES.svg arrived without one.
func TestLogoFor_CarriesAViewBox(t *testing.T) {
	for _, code := range []string{"SMILES", "AZUL", "LATAM", "GOL", "AA"} {
		logo := string(LogoFor(code, nil))
		if logo == "" {
			t.Errorf("%s: expected artwork", code)
			continue
		}
		if !strings.Contains(logo, "viewBox") {
			t.Errorf("%s: logo has no viewBox, so it will not scale in the PDF", code)
		}
	}
}

func TestPageHeight_GrowsWithTheContentAndStaysBounded(t *testing.T) {
	short := pageHeightMM(ViewModel{Groups: []Group{{Sections: []Section{{Lines: make([]Line, 3)}}}}})
	if short != pageMinMM {
		t.Errorf("a short document should sit at the minimum page: got %d, want %d", short, pageMinMM)
	}

	long := pageHeightMM(ViewModel{Groups: []Group{{Sections: []Section{{Lines: make([]Line, 60)}}}}})
	if long <= short {
		t.Errorf("a longer document needs a taller page: got %d, previous %d", long, short)
	}

	huge := pageHeightMM(ViewModel{Groups: []Group{{Sections: []Section{{Lines: make([]Line, 100000)}}}}})
	if huge != pageMaxMM {
		t.Errorf("the page must stay bounded: got %d, want %d", huge, pageMaxMM)
	}
}
