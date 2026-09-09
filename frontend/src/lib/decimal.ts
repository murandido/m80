// How the panel reads a number somebody typed.
//
// One grammar for money and for points, because the operator does not switch grammars
// between fields - and because the two used to disagree in a way that was silent and
// expensive: a dot was thrown away as a thousands separator, so "24.50" in a money field
// saved R$ 2.450,00 and "10.5" in the points field saved 105 milheiros. Nothing on screen
// said so.
//
// The rule, in one line: **a dot and a comma are both the decimal separator**. An operator
// on an English keyboard types "24.50" and one on a Brazilian keyboard types "24,50", and
// both mean the same amount. This is the same rule the grid's expression evaluator has
// always used (src/lib/expression.ts), which is where "189.22*2" and "189,22*2" already
// agree - so the two stop disagreeing.
//
// The one case with two readings is a lone separator followed by three digits: "1.500" is
// a thousand and a half to a Brazilian and one and a half to everybody else. It reads as
// the decimal, like every other lone separator, and the screens that take money show what
// the number resolves to while it is typed rather than leaving the operator to guess.
//
// Thousands grouping is still understood when it cannot be mistaken for anything else:
// when both characters appear, the last one is the decimal and the earlier ones group -
// "8.941,72" and "1,234.56" both read as eight thousand nine hundred and forty one point
// seventy two, and one thousand two hundred and thirty four point fifty six.

// Everything that is not a digit, a separator or a leading minus is dropped, so a value
// that arrives with its currency prefix ("R$ 24,50") or a stray space reads the same as one
// without.
const NOT_A_NUMBER = /[^\d.,-]/g

// parseDecimalInput reads typed text as a number, or null when there is no number in it -
// blank, "abc", a lone separator. Callers convert to their own unit afterwards.
export function parseDecimalInput(text: string): number | null {
  const cleaned = text.trim().replace(NOT_A_NUMBER, "")
  if (cleaned === "") return null

  // A minus only means anything at the front. One in the middle is a typo, not a
  // subtraction: the grid sends arithmetic to evaluateExpression before it ever gets here.
  const negative = cleaned.startsWith("-")
  const digitsAndSeparators = cleaned.replace(/-/g, "")

  const lastDot = digitsAndSeparators.lastIndexOf(".")
  const lastComma = digitsAndSeparators.lastIndexOf(",")

  let normalized: string
  if (lastDot >= 0 && lastComma >= 0) {
    // Both characters: the last one is the decimal, everything before it groups.
    const decimalAt = Math.max(lastDot, lastComma)
    const whole = digitsAndSeparators.slice(0, decimalAt).replace(/[.,]/g, "")
    const fraction = digitsAndSeparators.slice(decimalAt + 1).replace(/[.,]/g, "")
    normalized = `${whole}.${fraction}`
  } else {
    const separator = lastDot >= 0 ? "." : ","
    const parts = digitsAndSeparators.split(separator)
    // Repeated, it can only be grouping - "1.234.567" has no reading as a decimal.
    normalized =
      parts.length > 2 ? parts.join("") : parts.length === 2 ? `${parts[0]}.${parts[1]}` : parts[0]
  }

  // A lone separator is not a number. "12." is - it is one being typed, and reading it as
  // twelve keeps the field from refusing a value the operator can see on screen.
  if (normalized === "" || normalized === ".") return null

  const value = Number(normalized)
  if (Number.isNaN(value)) return null

  return negative ? -value : value
}

// sanitizeDecimalInput is the filter for a field while it is being typed: digits and at
// most one separator, keeping whichever character the operator reached for. Later ones are
// dropped without taking their digits with them, so holding a key down cannot silently
// change the value's magnitude.
export function sanitizeDecimalInput(raw: string): string {
  const cleaned = raw.replace(/[^\d.,]/g, "")
  const first = cleaned.search(/[.,]/)
  if (first < 0) return cleaned

  const separator = cleaned[first]
  const whole = cleaned.slice(0, first)
  const fraction = cleaned.slice(first + 1).replace(/[.,]/g, "")
  return `${whole}${separator}${fraction}`
}
