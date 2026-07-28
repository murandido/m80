package bot

import (
	"errors"
	"strings"
	"testing"
)

func TestIsAlteracaoMessage(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"ALTERACAO - IWBEQP", true},
		{"ALTERAÇÃO - IWBEQP", true},
		{"alteracao - iwbeqp", true},
		{"ALTERACAO - IWBEQP\n50k * 33 + 0 tx = 1.650,00", true},
		{"EMISSAO - SMILES - IWBEQP", false},
		{"vou alterar a emissão depois", false},
		{"", false},
	}

	for _, tt := range cases {
		if got := isAlteracaoMessage(tt.text); got != tt.want {
			t.Errorf("isAlteracaoMessage(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestParseAlteracao_ReadsTheLocatorAndClassifiesEveryLine(t *testing.T) {
	msg, err := parseAlteracao(strings.Join([]string{
		"ALTERACAO - iwbeqp",
		"50k * 33,00 + 100 tx = 1.750,00",
		"GRU - SILVA",
		"Murilo - Wilmara (AgenciaX)",
		"ACORDO",
	}, "\n"))
	if err != nil {
		t.Fatalf("parseAlteracao returned %v", err)
	}

	if msg.ProgramLocator != "IWBEQP" {
		t.Fatalf("ProgramLocator = %q, want the uppercased IWBEQP", msg.ProgramLocator)
	}

	want := []alteracaoLineKind{lineAmounts, lineRoute, lineOwners, lineObservation}
	if len(msg.Corrections) != len(want) {
		t.Fatalf("got %d correction line(s), want %d", len(msg.Corrections), len(want))
	}
	for i, kind := range want {
		if msg.Corrections[i].Kind != kind {
			t.Errorf("line %q classified as %v, want %v", msg.Corrections[i].Text, msg.Corrections[i].Kind, kind)
		}
	}
}

// Order carries no meaning: an operator fixing two things writes them in
// whatever order they thought of them.
func TestParseAlteracao_ClassificationDoesNotDependOnOrder(t *testing.T) {
	forward, err := parseAlteracao("ALTERACAO - ABC123\nGRU - SILVA\nMurilo")
	if err != nil {
		t.Fatalf("parseAlteracao returned %v", err)
	}
	backward, err := parseAlteracao("ALTERACAO - ABC123\nMurilo\nGRU - SILVA")
	if err != nil {
		t.Fatalf("parseAlteracao returned %v", err)
	}

	if forward.Corrections[0].Kind != backward.Corrections[1].Kind {
		t.Fatal("the route line was read differently depending on where it appeared")
	}
	if forward.Corrections[1].Kind != backward.Corrections[0].Kind {
		t.Fatal("the bare word was read differently depending on where it appeared")
	}
}

func TestClassifyAlteracaoLine(t *testing.T) {
	cases := []struct {
		line string
		want alteracaoLineKind
	}{
		{"50k * 33,00 + 100 tx = 1.750,00", lineAmounts},
		{"50k*33+0tx=1.650,00", lineAmounts},
		{"GRU", lineRoute},
		{"GRU - SILVA", lineRoute},
		{"Bia - Wilmara", lineRoute}, // provisional: settled against airports
		{"ACORDO", lineObservation},
		{"Murilo", lineObservation}, // provisional: settled against the presets
		{"Murilo - Wilmara", lineOwners},
		{"Murilo (AgenciaX)", lineOwners},
		// A parenthesized agency settles the line without the database, even
		// when the alias in front of it is three letters long.
		{"Ana (AgenciaX)", lineOwners},
	}

	for _, tt := range cases {
		if got := classifyAlteracaoLine(tt.line); got != tt.want {
			t.Errorf("classifyAlteracaoLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestParseAlteracao_Rejections(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		contains string
	}{
		{
			name:     "nothing to correct",
			text:     "ALTERACAO - IWBEQP",
			contains: "pelo menos uma linha",
		},
		{
			name:     "no locator",
			text:     "ALTERACAO\n50k * 33 + 0 tx = 1.650,00",
			contains: "A linha 1 precisa ser",
		},
		{
			name:     "empty locator",
			text:     "ALTERACAO - \n50k * 33 + 0 tx = 1.650,00",
			contains: "A linha 1 precisa ser",
		},
		{
			name:     "too many fields on line 1",
			text:     "ALTERACAO - SMILES - IWBEQP\n50k * 33 + 0 tx = 1.650,00",
			contains: "A linha 1 precisa ser",
		},
		{
			// The EMISSAO writes both locators as "PROGRAMA/CIA", so reaching for
			// the same notation here is a plausible mistake - and it is asking to
			// change the one field a correction may never touch.
			name:     "slash in the locator",
			text:     "ALTERACAO - IWBEQP/AA987654\n50k * 33 + 0 tx = 1.650,00",
			contains: "Não é possível alterar o PNR",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parseAlteracao(tt.text)
			if err == nil {
				t.Fatalf("parseAlteracao accepted %q as %+v", tt.text, msg)
			}

			// Every parse failure is the operator's typing, so all of them must be
			// safe to quote back into the group verbatim.
			var operator operatorError
			if !errors.As(err, &operator) {
				t.Fatalf("error %v is not an operatorError, so it would be answered generically", err)
			}
			if !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("error %q does not mention %q", err, tt.contains)
			}
		})
	}
}
