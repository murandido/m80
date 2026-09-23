package booking

import (
	"slices"
	"testing"
)

func TestSurnameCandidates(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"SILVA", []string{"SILVA"}},
		{"  silva ", []string{"SILVA"}},
		{"EDILHENE APARECIDA DA SILVA", []string{"SILVA", "APARECIDA", "EDILHENE"}},
		{"ANA MARIA DE SOUZA COSTA", []string{"COSTA", "SOUZA", "MARIA"}},
		{"LIMA LIMA", []string{"LIMA"}},
		{"", []string{""}},
	}
	for _, tt := range tests {
		if got := SurnameCandidates(tt.name); !slices.Equal(got, tt.want) {
			t.Errorf("SurnameCandidates(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestWholeSurnameCandidates(t *testing.T) {
	tests := map[string][]string{
		"DE OLIVEIRA RODRIGUES":               {"DE OLIVEIRA RODRIGUES", "RODRIGUES", "OLIVEIRA RODRIGUES"},
		"MARCO ANTONIO DEOLIVEIRARODRIGUES":   {"MARCO ANTONIO DEOLIVEIRARODRIGUES", "DEOLIVEIRARODRIGUES", "ANTONIO DEOLIVEIRARODRIGUES"},
		"marco antonio de oliveira rodrigues": {"MARCO ANTONIO DE OLIVEIRA RODRIGUES", "RODRIGUES", "OLIVEIRA RODRIGUES", "DE OLIVEIRA RODRIGUES"},
		"GIESE":                               {"GIESE"},
		"":                                    nil,
	}
	for name, want := range tests {
		if got := WholeSurnameCandidates(name, 4); !slices.Equal(got, want) {
			t.Errorf("WholeSurnameCandidates(%q) = %v, want %v", name, got, want)
		}
	}
}
