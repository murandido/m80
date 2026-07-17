// Live thousands-separator mask for plain integer fields (e.g. "Pontos"):
// strips non-digits and inserts "." every 3 digits from the right as the user
// types - same spirit as applyDateMask in date-mask.ts, just for numbers.
// No padding here on purpose: this runs on every keystroke of active typing,
// including Backspace - padding to a minimum digit count would re-assert
// itself on every deletion, making the field feel like it fights backspacing
// instead of shrinking naturally. Use applyPaddedThousandsMask for snapshots
// (read-only display, the value shown the instant a cell enters edit mode).
export function applyThousandsMask(raw: string): string {
  const digits = raw.replace(/\D/g, "")
  if (digits === "") return ""
  return digits.replace(/\B(?=(\d{3})+(?!\d))/g, ".")
}

// Same grouping, but left-pads to at least 4 digits first so values under
// 1000 always show the leading zero group too, e.g. 500 -> "0.500". Only
// safe for snapshots, not live typing - see applyThousandsMask's note.
export function applyPaddedThousandsMask(raw: string): string {
  const digits = raw.replace(/\D/g, "")
  if (digits === "") return ""
  const padded = digits.length < 4 ? digits.padStart(4, "0") : digits
  return padded.replace(/\B(?=(\d{3})+(?!\d))/g, ".")
}
