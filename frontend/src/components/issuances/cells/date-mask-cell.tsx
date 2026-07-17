"use client"

import { useEffect, useRef, useState } from "react"
import { useGridContext } from "@/components/issuances/grid-context"
import { Direction } from "@/lib/grid-navigation"
import { applyDateMask, isCompleteDateMask, isoDateToMasked, maskedDateToISO } from "@/lib/date-mask"

export function DateMaskCell({ r, c }: { r: number; c: number }) {
  const { rows, sel, editing, pendingChar, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } =
    useGridContext()
  const row = rows[r]
  const isThisCellEditing = editing && sel.r === r && sel.c === c
  const displayValue = row.flight_date ? isoDateToMasked(row.flight_date) : ""

  const [text, setText] = useState(displayValue)
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ? applyDateMask(pendingChar) : displayValue
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

    if (!isCompleteDateMask(text)) {
      onSaveAndMove(r, {}, direction)
      return
    }
    onSaveAndMove(r, { flight_date: maskedDateToISO(text) }, direction)
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
        className="block w-full truncate px-2 py-1.5 text-left"
      >
        {displayValue || <span className="text-muted-foreground">—</span>}
      </button>
    )
  }

  return (
    <input
      ref={inputRef}
      value={text}
      inputMode="numeric"
      placeholder="DD/MM/AAAA"
      onChange={(e) => setText(applyDateMask(e.target.value))}
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
      className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
    />
  )
}
