"use client"

import { useGridContext } from "@/components/issuances/grid-context"
import { formatCurrencyBRL } from "@/lib/currency"
import { CALCULATED_COLUMN_INDEX } from "@/lib/grid-navigation"

export function CalculatedCell({ r }: { r: number }) {
  const { rows, onCellClick } = useGridContext()
  const row = rows[r]

  const total =
    row.points !== null && row.value_of_the_thousand !== null
      ? Math.round((row.points / 1000) * row.value_of_the_thousand) + (row.taxes ?? 0)
      : null

  return (
    <button
      type="button"
      onClick={() => onCellClick(r, CALCULATED_COLUMN_INDEX)}
      className="block w-full truncate px-2 py-1.5 text-right italic text-muted-foreground"
    >
      {total !== null ? formatCurrencyBRL(total) : "—"}
    </button>
  )
}
