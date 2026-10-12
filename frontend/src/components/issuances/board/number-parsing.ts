import { centsFromReais, parseCurrencyBRLInput } from "@/lib/currency"
import { evaluateExpression, looksLikeExpression } from "@/lib/expression"
import {
  formatPointsInThousands,
  parsePointsFromThousands,
  pointsFromThousands,
} from "@/lib/points"

// The number fields of the board's form speak the grid's grammar (cells/number-cell.tsx):
// points in milheiros, money in reais with a comma or a dot, and either one may be a sum.
// Kept here rather than imported from the cell, which keeps it to itself.
export type NumberMode = "points" | "currency"

export function parseNumberText(text: string, mode: NumberMode): number | null {
  if (looksLikeExpression(text)) {
    const result = evaluateExpression(text)
    if (result === null) return null
    return mode === "points"
      ? pointsFromThousands(result)
      : centsFromReais(result)
  }
  return mode === "points"
    ? parsePointsFromThousands(text)
    : parseCurrencyBRLInput(text)
}

// What an input shows for a stored value: milheiros for points, reais for money, so
// typing over it and saving gives back the same number.
export function formatNumberText(
  value: number | null,
  mode: NumberMode,
): string {
  if (value === null) return ""
  if (mode === "points") return formatPointsInThousands(value)
  return (value / 100).toFixed(2).replace(".", ",")
}
