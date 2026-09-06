"use client"

import { memo, useEffect, useMemo, useRef, useState } from "react"
import { CellStateProps, useGridContext } from "@/components/issuances/grid-context"
import { DropdownPortal } from "@/components/issuances/cells/dropdown-portal"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"
import { centsFromReais, formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { evaluateExpression, looksLikeExpression } from "@/lib/expression"
import {
  applyPaddedThousandsMask,
  formatPointsInThousands,
  parsePointsFromThousands,
  pointsFromThousands,
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

// A cell whose text carries an operator is a sum to resolve, not a number to read, so the
// operator can add two taxes or split points across legs without reaching for a calculator.
// The result comes back in the unit the cell is typed in - milheiros for points, reais for
// money - so it takes the same conversion the plain parsers apply.
function parseInput(text: string, mode: "integer" | "currency"): number | null {
  if (looksLikeExpression(text)) {
    const result = evaluateExpression(text)
    if (result === null) return null
    return mode === "integer" ? pointsFromThousands(result) : centsFromReais(result)
  }
  return mode === "integer" ? parsePointsFromThousands(text) : parseCurrencyBRLInput(text)
}

export const NumberCell = memo(function NumberCell({ r, c, field, mode, row, isEditing, pendingChar }: NumberCellProps) {
  const { onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } = useGridContext()
  const isThisCellEditing = isEditing
  const rawValue = row[field]

  const [text, setText] = useState(() => formatEditBuffer(rawValue, mode))
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)

  // What the sum currently resolves to, shown under the input while it is being typed. Null
  // for a plain number (nothing to preview) and for a half-typed or malformed one, so the
  // hint appears exactly when the cell is about to save something other than what was typed
  // - which is also where the operator most wants to see it before committing.
  const preview = useMemo(
    () => (looksLikeExpression(text) ? parseInput(text, mode) : null),
    [text, mode],
  )

  useEffect(() => {
    if (isThisCellEditing) {
      // pendingChar carries a typed character or a whole pasted string. The points mask
      // steps aside for a sum, the same way it does on every later keystroke - otherwise
      // pasting "31+2,5" here would arrive as "312,5".
      const startValue = pendingChar
        ? mode === "integer" && !looksLikeExpression(pendingChar)
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

    const parsed = parseInput(text, mode)
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
    <>
      <input
        ref={inputRef}
        value={text}
        inputMode="decimal"
        title={
          mode === "integer"
            ? "Em milheiros: 10 = 10.000, 10,5 = 10.500. Aceita conta: 10+2,5"
            : "Aceita conta: 189,22+56,50"
        }
        onChange={(e) => {
          const raw = e.target.value
          // The points mask only allows digits and a comma, which would eat the operator the
          // moment it is typed. It steps aside once the text reads as a sum - tested on the
          // raw value, before the mask would have stripped what makes it one.
          const masked = mode === "integer" && !looksLikeExpression(raw) ? sanitizeThousandsInput(raw) : raw
          setText(masked)
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
      {preview !== null && (
        // Rendered through the portal for the same reason the autocomplete list is: an
        // absolutely positioned box inside a <td> can paint behind the neighbouring rows.
        <DropdownPortal anchorRef={inputRef}>
          <div className="px-2 py-1.5 text-right text-sm tabular-nums">
            = {formatDisplay(preview, mode)}
          </div>
        </DropdownPortal>
      )}
    </>
  )
})
