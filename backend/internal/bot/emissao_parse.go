package bot

import (
	"errors"
	"fmt"
	"strings"

	"github.com/murandido/m-80-backend/internal/utils"
)

// An EMISSAO message is what an operator types into the WhatsApp group right
// after issuing a ticket. It is not a command (it carries no "!" prefix) - it is
// the shorthand the team already used among themselves before the bot existed,
// which is why the format is dictated by habit rather than designed:
//
//	EMISSAO - SMILES - IWBEQP          line 1: program, airline, locators
//	CNF - MACHADO                      line 2: origin, plus what the airline demands
//	31k * 16,5 + 189,22 tx = 700,72    line 3: the numbers
//	Heraldo                             line 4: account, administrator, agency
//	ACORDO                             line 5: an observation preset code (optional)
//
// Everything here is pure string work: no database, no whatsmeow. What each
// field means - which airline "SMILES" defaults to, whose account "Heraldo" is -
// is resolved afterwards, in emissao_resolve.go.
type emissaoMessage struct {
	// ProgramCode is matched against loyalty_programs.code.
	ProgramCode string
	// AirlineCode is empty when line 1 named no airline, meaning the loyalty
	// program's default airline applies.
	AirlineCode string

	ProgramLocator string
	// AirlineLocator is empty unless line 1 carried a second, slash-separated
	// locator.
	AirlineLocator string

	OriginIATACode string
	// Line2Extras holds whatever followed the origin on line 2, in order. Their
	// meaning depends on airlines.requires_last_name and
	// requires_purchase_number, which the parser has no way of knowing - see
	// assignLine2Extras.
	Line2Extras []string

	Points             int
	ValueOfTheThousand int
	Taxes              int
	// Total is the amount the message itself claims, in cents. It is not stored
	// anywhere: it exists so a misread of any of the three numbers above can be
	// caught before it reaches the database. See validateTotal.
	Total int

	AccountNickname string
	// AdminNickname and AgencyNickname are empty when line 4 omitted them.
	AdminNickname  string
	AgencyNickname string

	// ObservationCode is empty when the message had no fifth line.
	ObservationCode string
}

// isEmissaoMessage reports whether a message looks like an EMISSAO at all. Kept
// deliberately cheap and allocation-light: every single message in every
// monitored group runs through it, and the overwhelming majority are ordinary
// conversation that must cost nothing.
func isEmissaoMessage(text string) bool {
	firstLine := text
	if cut := strings.IndexByte(text, '\n'); cut >= 0 {
		firstLine = text[:cut]
	}

	upper := strings.ToUpper(strings.TrimSpace(firstLine))
	return strings.HasPrefix(upper, "EMISSAO") || strings.HasPrefix(upper, "EMISSÃO")
}

// parseEmissao turns the raw message text into its parsed fields, or explains
// why it can't. Parsing never touches the database, so every failure here is
// about what the operator typed - which is why they are all operatorError and
// reach the group verbatim.
func parseEmissao(text string) (*emissaoMessage, error) {
	lines := splitNonEmptyLines(text)
	if len(lines) < 4 {
		return nil, operatorErrorf(nil, "Uma EMISSAO precisa de pelo menos 4 linhas, recebi %d", len(lines))
	}

	msg := &emissaoMessage{}

	if err := parseEmissaoHeader(lines[0], msg); err != nil {
		return nil, err
	}
	if err := parseEmissaoRoute(lines[1], msg); err != nil {
		return nil, err
	}
	if err := parseEmissaoAmounts(lines[2], msg); err != nil {
		return nil, err
	}
	if err := parseEmissaoOwners(lines[3], msg); err != nil {
		return nil, err
	}
	if len(lines) > 4 {
		msg.ObservationCode = strings.ToUpper(lines[4])
	}

	if err := validateTotal(msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// parseEmissaoHeader reads "EMISSAO - SMILES[/AA] - IWBEQP[/AA987654]".
func parseEmissaoHeader(line string, msg *emissaoMessage) error {
	parts := splitTrimmed(line, "-")
	if len(parts) != 3 {
		return operatorErrorf(nil, "A linha 1 precisa ser EMISSAO - PROGRAMA - LOCALIZADOR, recebi %q", line)
	}

	msg.ProgramCode, msg.AirlineCode = splitOptionalPair(parts[1])
	if msg.ProgramCode == "" {
		return operatorErrorf(nil, "A linha 1 não traz o programa: %q", line)
	}

	msg.ProgramLocator, msg.AirlineLocator = splitOptionalPair(parts[2])
	if msg.ProgramLocator == "" {
		return operatorErrorf(nil, "A linha 1 não traz o localizador: %q", line)
	}

	msg.ProgramCode = strings.ToUpper(msg.ProgramCode)
	msg.AirlineCode = strings.ToUpper(msg.AirlineCode)
	msg.ProgramLocator = strings.ToUpper(msg.ProgramLocator)
	msg.AirlineLocator = strings.ToUpper(msg.AirlineLocator)
	return nil
}

// parseEmissaoRoute reads "CNF[ - MACHADO][ - LA9574243YWPJ]". Only the origin
// is positionally fixed; what follows is handed on unassigned.
func parseEmissaoRoute(line string, msg *emissaoMessage) error {
	parts := splitTrimmed(line, "-")
	if len(parts) == 0 || parts[0] == "" {
		return operatorErrorf(nil, "A linha 2 não traz o aeroporto de origem: %q", line)
	}

	msg.OriginIATACode = strings.ToUpper(parts[0])
	if len(msg.OriginIATACode) != 3 {
		return operatorErrorf(nil, "%q não é um código de aeroporto de 3 letras", parts[0])
	}

	for _, extra := range parts[1:] {
		if extra != "" {
			msg.Line2Extras = append(msg.Line2Extras, strings.ToUpper(extra))
		}
	}
	return nil
}

// parseEmissaoAmounts reads "31k * 16,5 + 189,22 tx = 700,72". The tax term is
// optional - an award ticket can carry none - but the total never is: it is the
// only cross-check the message offers.
func parseEmissaoAmounts(line string, msg *emissaoMessage) error {
	const shape = "A linha 3 precisa ser PONTOS * MILHEIRO + TAXAS tx = TOTAL"

	amounts, total, found := strings.Cut(line, "=")
	if !found {
		return operatorErrorf(nil, "%s, recebi %q", shape, line)
	}

	var err error
	if msg.Total, err = utils.ParseBRLCents(total); err != nil {
		return amountError("O total", total, err)
	}

	// The tax term carries a trailing "tx" marker; without a "+" there simply is
	// no tax on this issuance.
	if before, taxes, hasTaxes := strings.Cut(amounts, "+"); hasTaxes {
		amounts = before
		taxes = strings.TrimSpace(taxes)
		if len(taxes) >= 2 && strings.EqualFold(taxes[len(taxes)-2:], "tx") {
			taxes = taxes[:len(taxes)-2]
		}
		if msg.Taxes, err = utils.ParseBRLCents(taxes); err != nil {
			return amountError("O valor das taxas", taxes, err)
		}
	}

	points, thousand, found := strings.Cut(amounts, "*")
	if !found {
		return operatorErrorf(nil, "%s, recebi %q", shape, line)
	}

	if msg.Points, err = parsePoints(points); err != nil {
		return amountError("A quantidade de pontos", points, err)
	}
	if msg.ValueOfTheThousand, err = utils.ParseBRLCents(thousand); err != nil {
		return amountError("O valor do milheiro", thousand, err)
	}
	return nil
}

// amountError phrases a bad number on line 3 for the operator reading it on
// their phone: which of the four fields it was, exactly what they typed, and
// what is wrong with it - "O valor do milheiro \"16,555\" está com casas decimais
// demais". The sentinels behind it stay in English, as developer-facing values in
// a general-purpose package should.
func amountError(label, value string, err error) error {
	problem := "não é um número válido"
	if errors.Is(err, utils.ErrTooManyDecimals) {
		problem = "está com casas decimais demais"
	}
	return operatorErrorf(nil, "%s %q %s", label, strings.TrimSpace(value), problem)
}

// parsePoints reads a point count, which is nearly always abbreviated with a "k"
// suffix - "31k", "89,912k", "360k" - and occasionally spelled out in full.
func parsePoints(value string) (int, error) {
	trimmed := strings.TrimSpace(value)

	if len(trimmed) > 0 && (trimmed[len(trimmed)-1] == 'k' || trimmed[len(trimmed)-1] == 'K') {
		return utils.ParseScaledDecimal(trimmed[:len(trimmed)-1], 3)
	}
	return utils.ParseScaledDecimal(trimmed, 0)
}

// parseEmissaoOwners reads "Heraldo[ - Fulano][ (AgenciaX)]".
func parseEmissaoOwners(line string, msg *emissaoMessage) error {
	// The agency is parenthesized, so it is lifted out before splitting the rest
	// - an agency name is free to contain a dash, unlike the nicknames.
	if open := strings.IndexByte(line, '('); open >= 0 {
		closing := strings.IndexByte(line[open:], ')')
		if closing < 0 {
			return operatorErrorf(nil, "A linha 4 abre um parêntese que nunca fecha: %q", line)
		}
		msg.AgencyNickname = strings.TrimSpace(line[open+1 : open+closing])
		line = line[:open] + line[open+closing+1:]
	}

	parts := splitTrimmed(line, "-")
	if len(parts) == 0 || parts[0] == "" {
		return operatorErrorf(nil, "A linha 4 não traz o apelido da conta: %q", line)
	}

	msg.AccountNickname = parts[0]
	if len(parts) > 1 {
		msg.AdminNickname = parts[1]
	}
	return nil
}

// validateTotal re-derives the total from the three numbers that will actually
// be stored and compares it against the one the operator typed. This is the
// safety net for the one genuinely ambiguous input the separator rules can't
// resolve - "1.500" is either fifteen hundred or one and a half - and for plain
// typos: a misread of any single field makes the sum disagree, and the message
// is refused instead of silently persisting a wrong price.
func validateTotal(msg *emissaoMessage) error {
	// Same arithmetic as the panel's own calculated column: round to the nearest
	// cent, then add the taxes untouched.
	expected := (msg.Points*msg.ValueOfTheThousand+500)/1000 + msg.Taxes

	if expected != msg.Total {
		return operatorErrorf(nil,
			"As contas da linha 3 não fecham: %s + %s = %s, mas a mensagem diz %s",
			formatCents((msg.Points*msg.ValueOfTheThousand+500)/1000),
			formatCents(msg.Taxes),
			formatCents(expected),
			formatCents(msg.Total),
		)
	}
	return nil
}

func formatCents(cents int) string {
	return fmt.Sprintf("%d,%02d", cents/100, cents%100)
}

// splitNonEmptyLines splits on newlines and drops blank ones, so a message
// padded with an extra line break still parses.
func splitNonEmptyLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// splitTrimmed splits and trims each field. The separator is matched without
// requiring the surrounding spaces, so "EMISSAO-SMILES-IWBEQP" reads the same as
// the spaced form - at the cost of nicknames not being able to contain a dash.
func splitTrimmed(line, sep string) []string {
	parts := strings.Split(line, sep)
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

// splitOptionalPair splits "SMILES/AA" into its two halves, returning an empty
// second half when there is no slash.
func splitOptionalPair(value string) (string, string) {
	first, second, _ := strings.Cut(value, "/")
	return strings.TrimSpace(first), strings.TrimSpace(second)
}
