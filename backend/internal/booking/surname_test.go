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
