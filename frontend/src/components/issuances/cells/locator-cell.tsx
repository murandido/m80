"use client"

import { memo, useEffect, useRef, useState } from "react"
import { CellStateProps, useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"

// Composite "Localizador" cell: same "/" convention as the Cia aérea field -
// text before the slash is program_locator, text after it (optional) is
// airline_locator (the PNR held directly with the operating airline).
//
// Airlines with requires_purchase_number (in practice LATAM) take a further
// segment for latam_purchase_number, the order number their booking is keyed on.
// It rides here rather than in a column of its own because a column would be
// empty on nearly every row, and because a second PNR is rare on exactly those
// airlines - so the two-segment form is read as locator + purchase number, and
// only the three-segment form spends a segment on the airline PNR:
//
//   ABC123                   locator
//   ABC123/9876543           locator + purchase number
//   ABC123/XYZ789/9876543    locator + airline PNR + purchase number
//   ABC123/XYZ789/           locator + airline PNR, purchase number cleared
//
// For every other airline nothing changes: one slash, and any further segment is
// ignored.
export const LocatorCell = memo(function LocatorCell({
  r,
  c,
  row,
  isEditing,
  pendingChar,
}: { r: number; c: number } & CellStateProps) {
  const { onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave, airlines } =
    useGridContext()
  const isThisCellEditing = isEditing

  const airline = airlines.find((candidate) => candidate.id === row.airline_id)
  const takesPurchaseNumber = airline?.requires_purchase_number ?? false

  // The "/" is only meaningful as typing syntax - kept in the edit buffer,
  // hidden from the read-only display (parentheses there instead).
  const editBufferValue = buildEditBuffer(row, takesPurchaseNumber)
  const readOnlyDisplay = buildReadOnlyDisplay(row, takesPurchaseNumber)

  const [value, setValue] = useState(editBufferValue)
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ?? editBufferValue
      setValue(startValue)
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

    // Every field this cell owns is sent even when empty: an omitted key reads as "keep
    // the previous value" on both the local merge and the PATCH, which made the fields
    // unclearable. latam_purchase_number is left out entirely for airlines that don't
    // use it, so a row that never had one is never told to clear it.
    onSaveAndMove(r, parseLocatorInput(value, takesPurchaseNumber), direction)
  }

  useEffect(() => {
    if (isThisCellEditing) {
      registerActiveCellSave(() => commit(null))
      return () => registerActiveCellSave(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing, value])

  if (!isThisCellEditing) {
    return (
      <button
        type="button"
        onClick={() => onCellClick(r, c)}
        className="block w-full truncate px-2 py-1.5 text-left"
      >
        {readOnlyDisplay || <span className="text-muted-foreground">—</span>}
      </button>
    )
  }

  return (
    <input
      ref={inputRef}
      value={value}
      placeholder={takesPurchaseNumber ? "localizador/nº compra" : "localizador/cia"}
      onChange={(e) => setValue(e.target.value)}
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
      className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
    />
  )
})

// parseLocatorInput splits the typed value into the fields it stands for.
// buildEditBuffer has to produce something this reads back the same way, and the
// trailing-slash case below is exactly where that bites.
function parseLocatorInput(value: string, takesPurchaseNumber: boolean): Partial<IssuanceRow> {
  const segments = value.split("/")
  const programLocator = segments[0]

  // No latam_purchase_number key at all here: an airline that doesn't use one has
  // nothing to clear, and sending "" would blank a value if the row's airline is
  // ever changed after the fact.
  if (!takesPurchaseNumber) {
    return {
      program_locator: programLocator,
      airline_locator: segments.slice(1).join("/").trim(),
    }
  }

  // Two segments mean locator + purchase number, because these airlines rarely have a
  // second PNR and the purchase number is what their booking is actually keyed on.
  // Spending the airline PNR slot takes the explicit three-segment form.
  if (segments.length < 2) {
    return { program_locator: programLocator, airline_locator: "", latam_purchase_number: "" }
  }
  if (segments.length === 2) {
    return {
      program_locator: programLocator,
      airline_locator: "",
      latam_purchase_number: segments[1].trim(),
    }
  }
  return {
    program_locator: programLocator,
    airline_locator: segments[1].trim(),
    latam_purchase_number: segments.slice(2).join("/").trim(),
  }
}

function buildEditBuffer(row: IssuanceRow, takesPurchaseNumber: boolean): string {
  if (!takesPurchaseNumber) {
    return row.program_locator + (row.airline_locator ? `/${row.airline_locator}` : "")
  }

  const purchaseNumber = row.latam_purchase_number ?? ""
  if (!row.airline_locator) {
    return purchaseNumber ? `${row.program_locator}/${purchaseNumber}` : row.program_locator
  }
  // The trailing slash is load-bearing when there is no purchase number: without it the
  // buffer would read back as locator + purchase number and silently move the airline
  // PNR into the wrong column the next time the operator saves the cell untouched.
  return `${row.program_locator}/${row.airline_locator}/${purchaseNumber}`
}

function buildReadOnlyDisplay(row: IssuanceRow, takesPurchaseNumber: boolean): string {
  let display = row.program_locator
  if (row.airline_locator) {
    display += ` (${row.airline_locator})`
  }
  if (takesPurchaseNumber && row.latam_purchase_number) {
    display += ` · nº ${row.latam_purchase_number}`
  }
  return display
}
