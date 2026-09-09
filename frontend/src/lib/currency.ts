import { parseDecimalInput } from "@/lib/decimal"

const brlFormatter = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
})

// value_of_the_thousand/taxes are stored as integer centavos (2450 = R$ 24,50).
export function formatCurrencyBRL(cents: number): string {
  return brlFormatter.format(cents / 100)
}

// Parses free-typed text into integer centavos, in the panel's one decimal grammar: a dot
// and a comma are both the decimal separator, so "24,50" and "24.50" are the same R$ 24,50
// (see src/lib/decimal.ts for why, and for what happens when both appear). Returns null for
// empty/unparseable input - callers should then skip the save, same laxity as the rest of
// the grid's editable fields.
export function parseCurrencyBRLInput(text: string): number | null {
  const value = parseDecimalInput(text)
  if (value === null) return null

  return centsFromReais(value)
}

// An amount already read as reais to the integer centavos the API stores. Split out of
// parseCurrencyBRLInput so a value that arrived some other way - the result of an expression
// typed into a cell, say - lands on the same rounding instead of its own.
export function centsFromReais(value: number): number {
  return Math.round(value * 100)
}
