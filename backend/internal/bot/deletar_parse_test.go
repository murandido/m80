package bot

import (
	"strings"
	"testing"
)

func TestIsDeletarMessage(t *testing.T) {
	cases := map[string]bool{
		"DELETAR - IWBEQP":    true,
		"deletar - iwbeqp":    true,
		"  DELETAR - X":       true,
		"CANCELAMENTO - X":    false,
		"ALTERACAO - IWBEQP":  false,
		"oi pessoal, bom dia": false,
	}
	for text, want := range cases {
		if got := isDeletarMessage(text); got != want {
			t.Errorf("isDeletarMessage(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestParseDeletar_Success(t *testing.T) {
	msg, err := parseDeletar("deletar - iwbeqp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ProgramLocator != "IWBEQP" {
		t.Fatalf("ProgramLocator = %q, want IWBEQP (uppercased)", msg.ProgramLocator)
	}
}

func TestParseDeletar_Rejections(t *testing.T) {
	cases := map[string]string{
		"":                              "uma única linha",
		"DELETAR - IWBEQP\nlinha extra": "uma única linha",
		"DELETAR":                       "linha precisa ser",
		"DELETAR - SMILES/IWBEQP":       "dois localizadores",
	}
	for text, mentions := range cases {
		t.Run(text, func(t *testing.T) {
			_, err := parseDeletar(text)
			if err == nil {
				t.Fatalf("expected an error for %q", text)
			}
			if got := err.Error(); !strings.Contains(got, mentions) {
				t.Fatalf("error = %q, want it to mention %q", got, mentions)
			}
		})
	}
}
