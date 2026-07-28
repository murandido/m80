package bot

import "strings"

// A DELETAR message undoes an operational mistake - a mistyped PNR - rather
// than a sale that fell through, which is what CANCELAMENTO is for. It carries
// nothing but the locator:
//
//	DELETAR - IWBEQP
type deletarMessage struct {
	// ProgramLocator identifies the issuance being deleted.
	ProgramLocator string
}

// isDeletarMessage reports whether a message looks like a DELETAR. Same cheap
// first-line check as the other three triggers - every message in every
// monitored group runs through it.
func isDeletarMessage(text string) bool {
	firstLine := text
	if cut := strings.IndexByte(text, '\n'); cut >= 0 {
		firstLine = text[:cut]
	}

	upper := strings.ToUpper(strings.TrimSpace(firstLine))
	return strings.HasPrefix(upper, "DELETAR")
}

// parseDeletar reads the locator. Unlike CANCELAMENTO there is no second line
// to speak of - DELETAR either finds the issuance or it doesn't.
func parseDeletar(text string) (*deletarMessage, error) {
	lines := splitNonEmptyLines(text)
	if len(lines) != 1 {
		return nil, operatorErrorf(nil, "Um DELETAR precisa ser uma única linha: DELETAR - LOCALIZADOR, recebi %d", len(lines))
	}

	locator, err := parseDeletarHeader(lines[0])
	if err != nil {
		return nil, err
	}
	return &deletarMessage{ProgramLocator: locator}, nil
}

// parseDeletarHeader reads "DELETAR - IWBEQP".
func parseDeletarHeader(line string) (string, error) {
	parts := splitTrimmed(line, "-")
	if len(parts) != 2 || parts[1] == "" {
		return "", operatorErrorf(nil, "A linha precisa ser DELETAR - LOCALIZADOR, recebi %q", line)
	}

	if strings.Contains(parts[1], "/") {
		return "", operatorErrorf(nil, "Não é possível informar dois localizadores num DELETAR")
	}

	return strings.ToUpper(parts[1]), nil
}
