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

// DD/MM/AAAA or DD/MM/AA -> YYYY-MM-DD, or null when the text is not a real date. A
// two-digit year is read as 20AA: operators type "26" for this year, and an issuance never
// falls in the 1900s. The day is checked against the month, so 31/02 is rejected instead of
// reaching the backend as a date Postgres refuses (or, worse, one that rolls into March).
export function parseMaskedDate(masked: string): string | null {
  const match = /^(\d{2})\/(\d{2})\/(\d{2}|\d{4})$/.exec(masked)
  if (!match) return null
  const [, day, month, typedYear] = match
  const year = typedYear.length === 2 ? `20${typedYear}` : typedYear
  const date = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)))
  // Date.UTC rolls overflow into the next month (31/02 -> 03/03), so a date that does not
  // read back as what was typed did not exist. Years below 100 are mapped to 19xx by
  // Date.UTC, which the year comparison also rejects ("0026" is not a year anyone means).
  if (
    date.getUTCFullYear() !== Number(year) ||
    date.getUTCMonth() !== Number(month) - 1 ||
    date.getUTCDate() !== Number(day)
  ) {
    return null
  }
  return `${year}-${month}-${day}`
}

// YYYY-MM-DD -> DD/MM/AAAA, for displaying a value that came from the backend.
export function isoDateToMasked(iso: string): string {
  const [year, month, day] = iso.slice(0, 10).split("-")
  return `${day}/${month}/${year}`
}
