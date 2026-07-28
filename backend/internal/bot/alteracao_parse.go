package bot

import (
	"strings"
	"unicode"
)

// An ALTERACAO message corrects an issuance that already exists. Unlike an
// EMISSAO it is partial - it carries only what was wrong - so its lines cannot
// be read by position:
//
//	ALTERACAO - IWBEQP                 line 1: the locator, which finds the issuance
//	50k * 33,00 + 100 tx = 1.750,00    the numbers
//	GRU - SILVA                        origin, and whatever the airline demands
//	Murilo - Wilmara (AgenciaX)       account, administrator, agency
//	ACORDO                             an observation preset code
//
// Everything after line 1 is optional and may come in any order, so each line is
// classified by its shape instead. Two of those shapes are genuinely
// indistinguishable from text alone - see alteracaoLineKind - so the kind the
// parser assigns them is provisional, and settleAlteracaoLines in
// alteracao_resolve.go has the final word.
type alteracaoMessage struct {
	// ProgramLocator identifies the issuance being corrected. It is the one
	// thing an ALTERACAO can never change.
	ProgramLocator string

	// Corrections holds every line after the first, in the order they were
	// written. Order is preserved only for error messages; it carries no
	// meaning.
	Corrections []alteracaoLine
}

type alteracaoLine struct {
	Text string
	Kind alteracaoLineKind
}

type alteracaoLineKind int

const (
	// lineAmounts is unmistakable: no other line multiplies.
	lineAmounts alteracaoLineKind = iota

	// lineRoute starts with a three-letter word, which is what an IATA code
	// looks like - and also what a short account alias looks like. "GRU" and
	// "Bia" are the same shape, so this one is provisional: the parser cannot
	// tell them apart, and settleAlteracaoLines asks the airports table.
	lineRoute

	// lineObservation is a single bare word, which is what a preset code looks
	// like - and also what an account alias looks like. "ACORDO" and "Murilo"
	// are the same shape, so this one is provisional too, settled against
	// observation_presets.
	lineObservation

	// lineOwners is everything else, and needs no help: an account line is the
	// only one that can carry a dash-separated administrator or a parenthesized
	// agency.
	lineOwners
)

func (k alteracaoLineKind) String() string {
	switch k {
	case lineAmounts:
		return "valores"
	case lineRoute:
		return "rota"
	case lineObservation:
		return "observação"
	default:
		return "conta"
	}
}

// isAlteracaoMessage reports whether a message looks like an ALTERACAO. Same
// cheapness requirement as isEmissaoMessage: every message in every monitored
// group runs through it.
func isAlteracaoMessage(text string) bool {
	firstLine := text
	if cut := strings.IndexByte(text, '\n'); cut >= 0 {
		firstLine = text[:cut]
	}

	upper := strings.ToUpper(strings.TrimSpace(firstLine))
	return strings.HasPrefix(upper, "ALTERACAO") || strings.HasPrefix(upper, "ALTERAÇÃO")
}

// parseAlteracao reads the locator and classifies every correction line. It
// never touches the database, so the two provisional kinds above may still be
// wrong here - settleAlteracaoLines finishes the job.
func parseAlteracao(text string) (*alteracaoMessage, error) {
	lines := splitNonEmptyLines(text)
	if len(lines) < 2 {
		return nil, operatorErrorf(nil, "Uma ALTERACAO precisa do localizador e de pelo menos uma linha para corrigir")
	}

	locator, err := parseAlteracaoHeader(lines[0])
	if err != nil {
		return nil, err
	}

	msg := &alteracaoMessage{ProgramLocator: locator}
	for _, line := range lines[1:] {
		msg.Corrections = append(msg.Corrections, alteracaoLine{Text: line, Kind: classifyAlteracaoLine(line)})
	}
	return msg, nil
}

// parseAlteracaoHeader reads "ALTERACAO - IWBEQP".
func parseAlteracaoHeader(line string) (string, error) {
	parts := splitTrimmed(line, "-")
	if len(parts) != 2 || parts[1] == "" {
		return "", operatorErrorf(nil, "A linha 1 precisa ser ALTERACAO - LOCALIZADOR, recebi %q", line)
	}

	// An EMISSAO writes both locators as "PROGRAMA/CIA", so an operator may well
	// reach for the same notation here. Refusing is clearer than silently
	// ignoring half of it: the locator is what finds the issuance, and the one
	// field a correction may never touch.
	if strings.Contains(parts[1], "/") {
		return "", operatorErrorf(nil, "Não é possível alterar o PNR de uma emissão já salva")
	}

	return strings.ToUpper(parts[1]), nil
}

// classifyAlteracaoLine decides what a correction line is about from its shape
// alone. The order of the tests is the order of certainty: the unmistakable
// shape first, then the two that need the database, then the catch-all.
func classifyAlteracaoLine(line string) alteracaoLineKind {
	if strings.Contains(line, "*") {
		return lineAmounts
	}

	fields := splitTrimmed(line, "-")
	if isThreeLetterWord(fields[0]) {
		return lineRoute
	}

	// A parenthesis means an agency, and a dash means an administrator; either
	// one settles the line as an account line without asking the database.
	if len(fields) == 1 && !strings.ContainsAny(line, "()") {
		return lineObservation
	}

	return lineOwners
}

func isThreeLetterWord(value string) bool {
	letters := 0
	for _, r := range value {
		if !unicode.IsLetter(r) {
			return false
		}
		letters++
	}
	return letters == 3
}
