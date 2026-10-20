import { parseDecimalInput, sanitizeDecimalInput } from "@/lib/decimal"

// How the panel reads and writes point counts.
//
// Operators quote points in milheiros - "dez" means ten thousand - and so does the bot, which
// accepts "31k" on a WhatsApp EMISSAO. The API stores the absolute count, so every screen that
// takes points typed by hand converts here instead of asking the operator to spell out the
// zeroes. The grid's Pontos cell and the cancellation dialog both read this one grammar; keeping
// two copies is how they drifted apart in the first place.

// Thousands-separator mask for a stored point count (e.g. "Pontos"): strips non-digits and
// inserts "." every 3 digits from the right, left-padding to at least 4 digits first so values
// under 1000 still show the leading zero group, e.g. 500 -> "0.500".
//
// Only safe for snapshots - a read-only display, or the value shown the instant a cell enters
// edit mode. On live typing the padding would re-assert itself on every Backspace, making the
// field feel like it fights being emptied.
export function applyPaddedThousandsMask(raw: string): string {
  const digits = raw.replace(/\D/g, "")
  if (digits === "") return ""
  const padded = digits.length < 4 ? digits.padStart(4, "0") : digits
  return padded.replace(/\B(?=(\d{3})+(?!\d))/g, ".")
}

// A signed point count in the padded grammar above, for the inventory screens (closing and the
// statement reconciliation) where a net movement can be negative. The mask itself only knows
// digits, so the sign is put back in front of it.
export function formatSignedPaddedPoints(points: number): string {
  const sign = points < 0 ? "-" : ""
  return sign + applyPaddedThousandsMask(String(Math.abs(points)))
}

// Digits and at most one separator, keeping whichever the operator reached for. No thousands
// grouping while typing: in milheiros the number has one or two digits before the separator,
// so a grouping dot would only get in the way - and a dot typed here is the decimal point,
// the same as everywhere else in the panel.
export function sanitizeThousandsInput(raw: string): string {
  return sanitizeDecimalInput(raw)
}

// A number already read as milheiros to the absolute point count the API stores. Split out of
// parsePointsFromThousands so a value that arrived some other way - the result of an
// expression typed into the cell, say - lands on the same rounding instead of its own.
export function pointsFromThousands(value: number): number {
  return Math.round(value * 1000)
}

// Text typed in milheiros to the absolute point count the API stores: "10" is ten thousand
// points and the separator is the fraction of a thousand, so "10,5" and "10.5" are both
// 10.500 and "0,5" is 500. Null for anything that is not a number - blank, "abc", a stray
// separator.
export function parsePointsFromThousands(text: string): number | null {
  const value = parseDecimalInput(text)
  if (value === null) return null
  return pointsFromThousands(value)
}

// The inverse, for seeding an input the operator is about to edit: a stored 10.500 has to come
// back as "10,5", because parsing "10.500" as milheiros would save ten and a half million.
export function formatPointsInThousands(points: number): string {
  return String(points / 1000).replace(".", ",")
}
