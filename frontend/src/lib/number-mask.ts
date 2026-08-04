// Thousands-separator mask for plain integer fields (e.g. "Pontos"): strips
// non-digits and inserts "." every 3 digits from the right, left-padding to at
// least 4 digits first so values under 1000 still show the leading zero group,
// e.g. 500 -> "0.500".
//
// Only safe for snapshots - a read-only display, or the value shown the instant a
// cell enters edit mode. On live typing the padding would re-assert itself on
// every Backspace, making the field feel like it fights being emptied.
export function applyPaddedThousandsMask(raw: string): string {
  const digits = raw.replace(/\D/g, "")
  if (digits === "") return ""
  const padded = digits.length < 4 ? digits.padStart(4, "0") : digits
  return padded.replace(/\B(?=(\d{3})+(?!\d))/g, ".")
}
