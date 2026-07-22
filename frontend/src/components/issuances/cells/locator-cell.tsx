"use client"

import { useEffect, useRef, useState } from "react"
import { useGridContext } from "@/components/issuances/grid-context"
import { Direction } from "@/lib/grid-navigation"

// Composite "Localizador" cell: same "/" convention as the Cia aérea field -
// text before the slash is program_locator, text after it (optional) is
// airline_locator (the PNR held directly with the operating airline).
export function LocatorCell({ r, c }: { r: number; c: number }) {
  const { rows, sel, editing, pendingChar, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } =
    useGridContext()
  const row = rows[r]
  const isThisCellEditing = editing && sel.r === r && sel.c === c
  // The "/" is only meaningful as typing syntax - kept in the edit buffer,
  // hidden from the read-only display (parentheses there instead).
  const editBufferValue = row.program_locator + (row.airline_locator ? `/${row.airline_locator}` : "")
  const readOnlyDisplay = row.airline_locator
    ? `${row.program_locator} (${row.airline_locator})`
    : row.program_locator

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

    const slashIndex = value.indexOf("/")
    const programLocator = slashIndex === -1 ? value : value.slice(0, slashIndex)
    const airlineLocator = slashIndex === -1 ? "" : value.slice(slashIndex + 1).trim()

    // airline_locator is sent even when empty: an omitted key reads as "keep the previous
    // value" on both the local merge and the PATCH, which made the field unclearable.
    onSaveAndMove(
      r,
      { program_locator: programLocator, airline_locator: airlineLocator },
      direction,
    )
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
      placeholder="localizador/cia"
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
}
