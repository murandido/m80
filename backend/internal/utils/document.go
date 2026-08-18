package utils

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// NormalizeCPF strips the mask off a Brazilian CPF and reports whether what is left is a
// real one. It returns the 11 bare digits.
//
// Rejecting invalid input matters more here than it usually does: the fingerprint derived
// from this string is a beneficiary's identity, so a CPF that is one digit off silently
// becomes a second person on the account and consumes a slot the program never charged.
func NormalizeCPF(raw string) (string, bool) {
	digits := make([]rune, 0, 11)
	for _, r := range raw {
		if unicode.IsDigit(r) {
			digits = append(digits, r)
		}
	}
	if len(digits) != 11 {
		return "", false
	}

	// Traveos hands back 00000000000 (and operators type 11111111111) when the document is
	// unknown. Those pass the check-digit rule below, so without this every passenger whose
	// document was not filled in would collapse into one beneficiary and the account would
	// look like it had used a single slot.
	allSame := true
	for _, r := range digits[1:] {
		if r != digits[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return "", false
	}

	if !validCPFCheckDigits(digits) {
		return "", false
	}
	return string(digits), true
}

// NormalizeCPFForIndex is the lenient sibling of NormalizeCPF, for the one job that cannot
// afford to be strict: keeping a CPF from being registered to two different people.
//
// The registry has always accepted whatever was typed - the old unique index was over the raw
// string - so some rows hold documents that fail the check digits. Fingerprinting only the
// valid ones would leave those rows with no fingerprint, no uniqueness, and a silent way to
// register the same person twice. So this normalizes rather than validates.
//
// For a valid CPF it returns exactly what NormalizeCPF returns, which is the point: a person
// and an account beneficiary holding the same document produce the same fingerprint, and the
// account holder is recognised without either side storing the number.
//
// Falls back to the trimmed original when there are no digits at all, so even junk keeps
// whatever uniqueness it had.
func NormalizeCPFForIndex(raw string) (string, bool) {
	digits := make([]rune, 0, 11)
	for _, r := range raw {
		if unicode.IsDigit(r) {
			digits = append(digits, r)
		}
	}
	if len(digits) > 0 {
		return string(digits), true
	}

	trimmed := strings.TrimSpace(raw)
	return trimmed, trimmed != ""
}

// validCPFCheckDigits applies the two mod-11 check digits the Receita Federal defines.
func validCPFCheckDigits(digits []rune) bool {
	for _, position := range []int{9, 10} {
		sum := 0
		weight := position + 1
		for i := 0; i < position; i++ {
			sum += int(digits[i]-'0') * weight
			weight--
		}
		expected := (sum * 10) % 11
		if expected == 10 {
			expected = 0
		}
		if expected != int(digits[position]-'0') {
			return false
		}
	}
	return true
}

// MaskCPF renders a normalized CPF the way the panel shows it: "***.456.789-**". The full
// number is never stored, so this is built at write time and kept alongside the fingerprint.
// The middle six digits are enough for a human to tell two beneficiaries apart and not
// enough to reconstruct the document.
func MaskCPF(normalized string) string {
	if len(normalized) != 11 {
		return ""
	}
	return "***." + normalized[3:6] + "." + normalized[6:9] + "-**"
}

// NormalizeName renders a passenger name as a match key: no accents, uppercase, single
// spaces. It is the fallback identity for a beneficiary who never arrived with a document -
// the common case, since the spreadsheet import writes nothing but a name.
//
// The SQL backfill in migration 37 reproduces this exact expression. If one changes, the
// other has to, or the first issuance after a deploy creates a duplicate of somebody the
// backfill already knew.
//
// Deliberately approximate. In the bot flow main_passenger_name is the surname the airline
// search used, so two siblings can collapse into one beneficiary and understate the count.
// That is why the beneficiary list is editable: the number informs a decision, it is not an
// audited fact.
func NormalizeName(raw string) string {
	withoutAccents, _, err := transform.String(
		transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC),
		raw,
	)
	if err != nil {
		// The transform only fails on malformed UTF-8; the raw name still beats nothing.
		withoutAccents = raw
	}
	return strings.ToUpper(strings.Join(strings.Fields(withoutAccents), " "))
}
