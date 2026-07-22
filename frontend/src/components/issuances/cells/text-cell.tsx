"use client"

import { useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"

interface TextCellProps {
  r: number
  c: number
  field: "main_passenger_name"
}

export function TextCell({ r, c, field }: TextCellProps) {
  const { rows, sel, editing, pendingChar, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } =
    useGridContext()
  const row = rows[r]
  const isThisCellEditing = editing && sel.r === r && sel.c === c
  const initialValue = row[field] ?? ""

  const [value, setValue] = useState(initialValue)
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ?? initialValue
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
    onSaveAndMove(r, { [field]: value } as Partial<IssuanceRow>, direction)
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
        {initialValue || <span className="text-muted-foreground">—</span>}
      </button>
    )
  }

  return (
    <input
      ref={inputRef}
      value={value}
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
      className={cn(
        "w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary",
      )}
    />
  )
}
