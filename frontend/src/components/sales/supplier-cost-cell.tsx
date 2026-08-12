"use client"

import { useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"

interface SupplierCostCellProps {
  // Centavos, or null while the cost has not been agreed with the administrator.
  value: number | null
  onCommit: (cents: number | null) => void
  saving?: boolean
  disabled?: boolean
}

// One editable cost.
//
// The draft only exists while the cell is being edited; the rest of the time the input
// renders straight from the prop. That is what lets a bulk apply, an SSE-driven refresh or
// a rejected save all land on a cell nobody is typing into, with no effect synchronising
// two copies of the same value. It also means the cell never shows a figure the server has
// not confirmed - on a failed save it snaps back to the real one, next to the error toast.
export function SupplierCostCell({ value, onCommit, saving, disabled }: SupplierCostCellProps) {
  const [draft, setDraft] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  // Escape has to reach the blur that follows it, and a setState would not be visible to
  // that handler in time - it reads the draft from the render it was created in.
  const cancelled = useRef(false)

  const formatted = value === null ? "" : formatCurrencyBRL(value)

  function commit() {
    const current = draft
    setDraft(null)
    if (cancelled.current) {
      cancelled.current = false
      return
    }
    if (current === null) return

    const trimmed = current.trim()
    const parsed = trimmed === "" ? null : parseCurrencyBRLInput(trimmed)

    // Unparseable input is discarded rather than saved as something the operator did not
    // mean. Clearing the field, on the other hand, is a real instruction: it puts the
    // issuance back to "cost not agreed", which is what makes the PDF refuse again.
    if (trimmed !== "" && parsed === null) return
    if (parsed === value) return

    onCommit(parsed)
  }

  return (
    <input
      ref={inputRef}
      value={draft ?? formatted}
      disabled={disabled}
      inputMode="decimal"
      placeholder="—"
      aria-label="Milheiro de custo"
      onFocus={(e) => {
        // Strip the R$ while editing: parseCurrencyBRLInput copes with it, but selecting
        // over a prefix to retype a number is needless friction.
        setDraft(value === null ? "" : String(value / 100).replace(".", ","))
        e.currentTarget.select()
      }}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault()
          inputRef.current?.blur()
        }
        if (e.key === "Escape") {
          e.preventDefault()
          cancelled.current = true
          inputRef.current?.blur()
        }
      }}
      className={cn(
        "w-24 rounded-sm border bg-transparent px-1.5 py-0.5 text-right text-sm tabular-nums",
        "focus:border-slate-400 focus:bg-white focus:outline-none",
        // An unset cost is the thing the operator is here to fix, so it is marked rather
        // than left looking like an empty column.
        value === null
          ? "border-amber-300 bg-amber-50 placeholder:text-amber-600"
          : "border-transparent hover:border-slate-300",
        saving && "opacity-60",
        disabled && "cursor-not-allowed opacity-50",
      )}
    />
  )
}
