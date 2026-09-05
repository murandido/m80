"use client"

import { memo, useEffect, useRef, useState } from "react"
import { CellStateProps, useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import {
  applyPaddedThousandsMask,
  formatPointsInThousands,
  parsePointsFromThousands,
  sanitizeThousandsInput,
} from "@/lib/points"

interface NumberCellProps extends CellStateProps {
  r: number
  c: number
  field: "points" | "value_of_the_thousand" | "taxes" | "redemption_fee_saved_cents"
  mode: "integer" | "currency"
}

function formatDisplay(value: number | null, mode: "integer" | "currency"): string {
  if (value === null) return ""
  return mode === "currency" ? formatCurrencyBRL(value) : applyPaddedThousandsMask(String(value))
}

// The points column is typed in milheiros, so the edit buffer has to be in milheiros
// too - seeding it with the stored "10.000" and parsing it back would save ten
// million the moment somebody re-enters a cell they did not mean to change.
function formatEditBuffer(value: number | null, mode: "integer" | "currency"): string {
  if (value === null) return ""
  if (mode === "integer") return formatPointsInThousands(value)
  return (value / 100).toFixed(2).replace(".", ",")
}

export const NumberCell = memo(function NumberCell({ r, c, field, mode, row, isEditing, pendingChar }: NumberCellProps) {
  const { onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } = useGridContext()
  const isThisCellEditing = isEditing
  const rawValue = row[field]

  const [text, setText] = useState(() => formatEditBuffer(rawValue, mode))
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar
        ? mode === "integer"
          ? sanitizeThousandsInput(pendingChar)
          : pendingChar
        : formatEditBuffer(rawValue, mode)
      setText(startValue)
      savedRef.current = false
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.setSelectionRange(startValue.length, startValue.length)
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing])

  function commit(direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true

    // An emptied cell clears the field, the same thing Delete on the selected cell does.
    // Only a blank buffer counts: text that simply fails to parse ("abc", a stray "-")
    // is a typo, and wiping the stored value over one would lose data the operator never
    // meant to touch.
    if (text.trim() === "") {
      onSaveAndMove(r, { [field]: null } as Partial<IssuanceRow>, direction)
      return
    }

    const parsed = mode === "integer" ? parsePointsFromThousands(text) : parseCurrencyBRLInput(text)
    if (parsed === null) {
      onSaveAndMove(r, {}, direction)
      return
    }
    onSaveAndMove(r, { [field]: parsed } as Partial<IssuanceRow>, direction)
  }

  useEffect(() => {
    if (isThisCellEditing) {
      registerActiveCellSave(() => commit(null))
      return () => registerActiveCellSave(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing, text])

  if (!isThisCellEditing) {
    return (
      <button
        type="button"
        onClick={() => onCellClick(r, c)}
        className="block w-full truncate px-2 py-1.5 text-right"
      >
        {rawValue !== null ? formatDisplay(rawValue, mode) : <span className="text-muted-foreground">—</span>}
      </button>
    )
  }

  return (
    <input
      ref={inputRef}
      value={text}
      inputMode="decimal"
      title={mode === "integer" ? "Em milheiros: 10 = 10.000, 10,5 = 10.500" : undefined}
      onChange={(e) => {
        const raw = e.target.value
        setText(mode === "integer" ? sanitizeThousandsInput(raw) : raw)
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault()
          onExitEdit()
        } else if (e.key === "Enter") {
          e.preventDefault()
          commit(e.shiftKey ? "up" : "down")
        } else if (e.key === "Tab") {
          e.preventDefault()
          commit(e.shiftKey ? "tab-left" : "tab-right")
        } else if (e.key === "ArrowDown") {
          e.preventDefault()
          commit("down")
        } else if (e.key === "ArrowUp") {
          e.preventDefault()
          commit("up")
        }
      }}
      onBlur={() => commit(null)}
      className="w-full bg-transparent px-2 py-1.5 text-right outline-none ring-2 ring-primary"
    />
  )
})
