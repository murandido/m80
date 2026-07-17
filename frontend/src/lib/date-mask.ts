// Formats raw digit input into the DD/MM/AAAA mask: strips non-digits, caps at
// 8 digits, auto-inserts slashes after position 2 and 4. Doesn't validate real
// dates (day 32, month 13 aren't blocked) - matches the prototype's laxity.
export function applyDateMask(raw: string): string {
  const digits = raw.replace(/\D/g, "").slice(0, 8)
  const day = digits.slice(0, 2)
  const month = digits.slice(2, 4)
  const year = digits.slice(4, 8)
  return [day, month, year].filter(Boolean).join("/")
}

export function isCompleteDateMask(masked: string): boolean {
  return /^\d{2}\/\d{2}\/\d{4}$/.test(masked)
}

// DD/MM/AAAA -> YYYY-MM-DD. Only call once isCompleteDateMask() is true.
export function maskedDateToISO(masked: string): string {
  const [day, month, year] = masked.split("/")
  return `${year}-${month}-${day}`
}

// YYYY-MM-DD -> DD/MM/AAAA, for displaying a value that came from the backend.
export function isoDateToMasked(iso: string): string {
  const [year, month, day] = iso.slice(0, 10).split("-")
  return `${day}/${month}/${year}`
}
