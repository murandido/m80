const brlFormatter = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
})

// value_of_the_thousand/taxes are stored as integer centavos (2450 = R$ 24,50).
export function formatCurrencyBRL(cents: number): string {
  return brlFormatter.format(cents / 100)
}

// Parses free-typed text into integer centavos. Comma is treated as the
// decimal separator (Brazilian convention, e.g. "24,50"); dots are stripped as
// thousand separators. A bare integer with no comma ("1500") is read as whole
// reais. Returns null for empty/unparseable input - callers should then skip
// the save, same laxity as the rest of the grid's editable fields.
export function parseCurrencyBRLInput(text: string): number | null {
  const trimmed = text.trim()
  if (trimmed === "") return null

  const normalized = trimmed.replace(/\./g, "").replace(",", ".")
  const value = Number(normalized)
  if (Number.isNaN(value)) return null

  return centsFromReais(value)
}

// An amount already read as reais to the integer centavos the API stores. Split out of
// parseCurrencyBRLInput so a value that arrived some other way - the result of an expression
// typed into a cell, say - lands on the same rounding instead of its own.
export function centsFromReais(value: number): number {
  return Math.round(value * 100)
}
