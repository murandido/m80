"use client"

import { IssuanceRow } from "@/components/issuances/types"
import { formatCurrencyBRL } from "@/lib/currency"

/**
 * How much redemption fee the rows currently on screen avoided by being issued on exempt
 * accounts - Azul's Diamante Unique. It is the number that prices choosing one of those
 * accounts over an ordinary one.
 *
 * Summed over what is rendered rather than fetched as a period total on purpose: the screen
 * loads the whole filtered period at once, so this always describes exactly the rows the
 * operator is looking at, and it follows a filter change with no extra request.
 *
 * Rows on ordinary accounts contribute nothing and are not counted as anything - what they
 * paid the airline is settled outside this system and is deliberately not recorded.
 */
export function RedemptionFeeSummary({ rows }: { rows: IssuanceRow[] }) {
  let saved = 0
  let rowsWithSaving = 0

  for (const row of rows) {
    if (row.isDraft || row.redemption_fee_saved_cents === null) continue
    saved += row.redemption_fee_saved_cents
    rowsWithSaving++
  }

  if (rowsWithSaving === 0) return null

  return (
    <div className="flex flex-wrap items-center justify-end gap-x-4 gap-y-1 px-2 py-2 text-xs text-slate-500">
      <span>
        Taxa de resgate economizada:{" "}
        <span className="font-medium text-emerald-700">{formatCurrencyBRL(saved)}</span>
      </span>
      <span>
        {rowsWithSaving === 1 ? "1 emissão isenta" : `${rowsWithSaving} emissões isentas`}
      </span>
    </div>
  )
}
