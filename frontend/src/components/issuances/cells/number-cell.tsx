"use client"

import { useEffect, useRef, useState } from "react"
import { useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { applyThousandsMask, applyPaddedThousandsMask } from "@/lib/number-mask"

interface NumberCellProps {
  r: number
  c: number
  field: "points" | "value_of_the_thousand" | "taxes"
  mode: "integer" | "currency"
}

function formatDisplay(value: number | null, mode: "integer" | "currency"): string {
  if (value === null) return ""
  return mode === "currency" ? formatCurrencyBRL(value) : applyPaddedThousandsMask(String(value))
}

function formatEditBuffer(value: number | null, mode: "integer" | "currency"): string {
  if (value === null) return ""
  if (mode === "integer") return applyPaddedThousandsMask(String(value))
  return (value / 100).toFixed(2).replace(".", ",")
}

export function NumberCell({ r, c, field, mode }: NumberCellProps) {
  const { rows, sel, editing, pendingChar, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } =
    useGridContext()
  const row = rows[r]
  const isThisCellEditing = editing && sel.r === r && sel.c === c
  const rawValue = row[field]

  const [text, setText] = useState(() => formatEditBuffer(rawValue, mode))
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar
        ? mode === "integer"
          ? applyThousandsMask(pendingChar)
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

    const parsed = mode === "integer" ? parseIntOrNull(text) : parseCurrencyBRLInput(text)
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
      onChange={(e) => {
        const raw = e.target.value
        // Let a typed comma sit as-is instead of stripping it live (habit
        // carried over from the currency fields) - it gets cleaned up when
        // the cell is committed, via parseIntOrNull below.
        setText(mode === "integer" && !raw.includes(",") ? applyThousandsMask(raw) : raw)
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault()
          onExitEdit()
        } else if (e.key === "Enter") {
          e.preventDefault()
          commit("tab-right")
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
}

function parseIntOrNull(text: string): number | null {
  const trimmed = text.trim()
  if (trimmed === "") return null
  const value = Number(trimmed.replace(/\D/g, ""))
  return Number.isNaN(value) ? null : value
}
