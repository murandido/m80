package ticket

import (
	"regexp"
	"strconv"
	"strings"
)

// The cabin arrives dirty from the vendor and is printed on a document the
// passenger travels with, so it is cleaned here - at display time - and never
// on the way into the database. issuance_flights.cabin keeps what Traveos
// actually said; rewriting it there would be inventing data, and that decision
// is already recorded in internal/issuance/traveos_map.go and in
// docs/traveos-api.md §5.

// orphanEscape matches a unicode escape that lost its backslash somewhere
// inside Traveos - "Econu00f4mica" is the observed case, and it reached a
// customer's PDF spelled exactly that way.
//
// The callback additionally requires at least one digit among the four hex
// characters. Without that guard the pattern would also match ordinary prose (a
// "u" followed by four letters that happen to be a-f), and turning a real word
// into a stray rune is a worse failure than leaving one escape unfixed. Every
// escape the vendor produces is of the form u00XX, so the guard costs nothing.
var orphanEscape = regexp.MustCompile(`u[0-9a-fA-F]{4}`)

// cabinTranslations maps the vocabulary we have seen onto the Portuguese the
// ticket is read in. The key is the label already lowercased and stripped of
// accents, so "Econômica", "economica" and "ECONOMICA" all land on the same
// entry.
//
// A label that is not in here passes through untouched: the dictionary is a
// courtesy, not a filter. A cabin nobody has seen before has to reach the
// passenger the way the airline wrote it, not disappear behind a fallback.
var cabinTranslations = map[string]string{
	"economica":         "Econômica",
	"economy":           "Econômica",
	"premium economy":   "Econômica Premium",
	"economy premium":   "Econômica Premium",
	"premium economica": "Econômica Premium",
	"economica premium": "Econômica Premium",
	"business":          "Executiva",
	"executiva":         "Executiva",
	"first":             "Primeira",
	"first class":       "Primeira",
	"primeira":          "Primeira",
	"primeira classe":   "Primeira",
}

// cleanCabin turns whatever the vendor sent into what the ticket prints.
func cleanCabin(cabin string) string {
	cabin = decodeOrphanEscapes(strings.TrimSpace(cabin))
	if cabin == "" {
		return ""
	}

	if translated, ok := cabinTranslations[normaliseCabinKey(cabin)]; ok {
		return translated
	}
	return cabin
}

func decodeOrphanEscapes(cabin string) string {
	return orphanEscape.ReplaceAllStringFunc(cabin, func(match string) string {
		digits := match[1:]
		if !strings.ContainsAny(digits, "0123456789") {
			return match
		}
		// Put the backslash back and let the standard library do the decoding,
		// rather than parsing the hex and converting to a rune by hand. The
		// escape is exactly the one Go's own syntax uses, so this is the same
		// operation the vendor failed to perform.
		decoded, err := strconv.Unquote(`"\u` + digits + `"`)
		if err != nil {
			return match
		}
		return decoded
	})
}

// normaliseCabinKey lowercases, drops accents and collapses whitespace, so the
// dictionary needs one entry per concept rather than one per spelling.
func normaliseCabinKey(cabin string) string {
	var builder strings.Builder
	builder.Grow(len(cabin))

	for _, r := range strings.ToLower(cabin) {
		if folded, ok := accentFolds[r]; ok {
			builder.WriteRune(folded)
			continue
		}
		builder.WriteRune(r)
	}

	return strings.Join(strings.Fields(builder.String()), " ")
}

// accentFolds covers the Portuguese and Spanish accents an airline might write
// a cabin name with. A full Unicode normalisation (golang.org/x/text) would
// pull a dependency in for twelve runes.
var accentFolds = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a',
	'é': 'e', 'ê': 'e', 'è': 'e', 'ë': 'e',
	'í': 'i', 'î': 'i', 'ì': 'i', 'ï': 'i',
	'ó': 'o', 'ô': 'o', 'õ': 'o', 'ò': 'o', 'ö': 'o',
	'ú': 'u', 'û': 'u', 'ù': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}
