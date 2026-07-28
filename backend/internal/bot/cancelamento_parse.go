package bot

import (
	"strings"

	"github.com/murandido/m-80-backend/internal/utils"
)

// A CANCELAMENTO message cancels an issuance that already exists. Unlike an
// ALTERACAO it never touches the issuance's own fields - it only records what
// comes back:
//
//	CANCELAMENTO - IWBEQP    line 1: the locator, which finds the issuance
//	0k + 50 tx               line 2: what comes back - optional
//
// Line 2 is the one genuinely custom shape here: when it is missing the
// cancellation is a full refund (everything the issuance charged comes back,
// no fee); when it is present the points term is mandatory - even "0k" - so a
// message never reads as "forgot to fill this in" versus "meant zero".
type cancelamentoMessage struct {
	// ProgramLocator identifies the issuance being cancelled.
	ProgramLocator string

	// HasRefundLine is false when the message carried no line 2 at all - the
	// caller then defaults refunded_points/refunded_taxes to whatever the
	// issuance originally charged, since parsing has no access to that row.
	HasRefundLine bool

	// RefundedPoints is only meaningful when HasRefundLine is true - it is
	// always present on that line, even when the answer is zero.
	RefundedPoints int
	// RefundedTaxes and CancellationFee are independent of one another and of
	// RefundedPoints - each is nil when its own term was absent from line 2.
	RefundedTaxes   *int
	CancellationFee *int
}

// isCancelamentoMessage reports whether a message looks like a CANCELAMENTO.
// Same cheap first-line check as isEmissaoMessage/isAlteracaoMessage - every
// message in every monitored group runs through it.
func isCancelamentoMessage(text string) bool {
	firstLine := text
	if cut := strings.IndexByte(text, '\n'); cut >= 0 {
		firstLine = text[:cut]
	}

	upper := strings.ToUpper(strings.TrimSpace(firstLine))
	return strings.HasPrefix(upper, "CANCELAMENTO")
}

// parseCancelamento reads the locator and, when present, the refund line.
func parseCancelamento(text string) (*cancelamentoMessage, error) {
	lines := splitNonEmptyLines(text)
	if len(lines) == 0 {
		return nil, operatorErrorf(nil, "Um CANCELAMENTO precisa pelo menos do localizador")
	}
	if len(lines) > 2 {
		return nil, operatorErrorf(nil,
			"Um CANCELAMENTO aceita no máximo 2 linhas (localizador e o que volta), recebi %d", len(lines))
	}

	locator, err := parseCancelamentoHeader(lines[0])
	if err != nil {
		return nil, err
	}

	msg := &cancelamentoMessage{ProgramLocator: locator}
	if len(lines) == 2 {
		if err := parseCancelamentoRefund(lines[1], msg); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

// parseCancelamentoHeader reads "CANCELAMENTO - IWBEQP".
func parseCancelamentoHeader(line string) (string, error) {
	parts := splitTrimmed(line, "-")
	if len(parts) != 2 || parts[1] == "" {
		return "", operatorErrorf(nil, "A linha 1 precisa ser CANCELAMENTO - LOCALIZADOR, recebi %q", line)
	}

	// Same reasoning as ALTERACAO's header: the locator finds the issuance and
	// is never something a cancellation can touch.
	if strings.Contains(parts[1], "/") {
		return "", operatorErrorf(nil, "Não é possível informar dois localizadores num CANCELAMENTO")
	}

	return strings.ToUpper(parts[1]), nil
}

// parseCancelamentoRefund reads "0k + 50 tx" / "30k + 50 multa" - what comes
// back from the cancellation. The points term is positional (always first);
// "tx" and "multa" may each appear at most once, in either order, and both
// are optional.
func parseCancelamentoRefund(line string, msg *cancelamentoMessage) error {
	msg.HasRefundLine = true

	terms := strings.Split(line, "+")

	points, err := parsePoints(terms[0])
	if err != nil {
		return operatorErrorf(err, "Os pontos que voltam %q não são um número válido", strings.TrimSpace(terms[0]))
	}
	msg.RefundedPoints = points

	for _, term := range terms[1:] {
		term = strings.TrimSpace(term)

		switch {
		case len(term) > 2 && strings.EqualFold(term[len(term)-2:], "tx"):
			if msg.RefundedTaxes != nil {
				return operatorErrorf(nil, "A linha 2 informa a taxa duas vezes: %q", line)
			}
			cents, err := utils.ParseBRLCents(term[:len(term)-2])
			if err != nil {
				return operatorErrorf(err, "O valor da taxa %q não é um número válido", strings.TrimSpace(term[:len(term)-2]))
			}
			msg.RefundedTaxes = &cents

		case len(term) > 5 && strings.EqualFold(term[len(term)-5:], "multa"):
			if msg.CancellationFee != nil {
				return operatorErrorf(nil, "A linha 2 informa a multa duas vezes: %q", line)
			}
			cents, err := utils.ParseBRLCents(term[:len(term)-5])
			if err != nil {
				return operatorErrorf(err, "O valor da multa %q não é um número válido", strings.TrimSpace(term[:len(term)-5]))
			}
			msg.CancellationFee = &cents

		default:
			return operatorErrorf(nil,
				`Não entendi %q na linha 2 - depois dos pontos, use "tx" para taxa ou "multa" para multa`, term)
		}
	}
	return nil
}
