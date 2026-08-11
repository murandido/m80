"use client"

import { memo } from "react"
import { CellStateProps, useGridContext } from "@/components/issuances/grid-context"
import { issuanceTotalCents } from "@/components/issuances/emissao-message"
import { formatCurrencyBRL } from "@/lib/currency"
import { CALCULATED_COLUMN_INDEX } from "@/lib/grid-navigation"

export const CalculatedCell = memo(function CalculatedCell({
  r,
  row,
}: { r: number } & Pick<CellStateProps, "row">) {
  const { onCellClick } = useGridContext()

  // Shared with the copied EMISSAO message, which the bot checksums against this exact
  // number - see issuanceTotalCents.
  const total = issuanceTotalCents(row)

  return (
    <button
      type="button"
      onClick={() => onCellClick(r, CALCULATED_COLUMN_INDEX)}
      className="block w-full truncate px-2 py-1.5 text-right italic text-muted-foreground"
    >
      {total !== null ? formatCurrencyBRL(total) : "—"}
    </button>
  )
})
