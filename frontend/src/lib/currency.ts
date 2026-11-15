import { parseDecimalInput, roundScaled } from "@/lib/decimal"

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
  return roundScaled(value, 100)
}

// What a number of points is worth at a price per thousand (centavos), rounded up to the
// cent. The same rule as sale.PointsValueCents in the backend, which is what the payments
// screen charges: the board's total, the copied EMISSAO message and the payments screen all
// have to show the same centavo for the same issuance. A zero or negative product truncates
// toward zero, as the Go function does.
export function pointsValueCents(points: number, valueOfTheThousand: number): number {
  const product = points * valueOfTheThousand
  if (product <= 0) return Math.trunc(product / 1000)
  return Math.ceil(product / 1000)
}
