"use client"

import { useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { useGridContext } from "@/components/issuances/grid-context"
import { IssuanceStatus } from "@/schemas/issuance"
import { Direction } from "@/lib/grid-navigation"

const STATUS_LABELS: Record<IssuanceStatus, string> = {
  incomplete: "Incompleto",
  complete: "Completo",
  cancelled: "Cancelado",
}

const STATUS_OPTIONS: IssuanceStatus[] = ["incomplete", "complete", "cancelled"]

export function StatusCell({ r, c }: { r: number; c: number }) {
  const { rows, sel, editing, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } = useGridContext()
  const row = rows[r]
  const isThisCellEditing = editing && sel.r === r && sel.c === c

  const [value, setValue] = useState<IssuanceStatus>(row.status)
  const selectRef = useRef<HTMLSelectElement>(null)
  const savedRef = useRef(false)

  useEffect(() => {
    if (isThisCellEditing) {
      setValue(row.status)
      savedRef.current = false
      requestAnimationFrame(() => selectRef.current?.focus())
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing])

  function commit(direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true
    onSaveAndMove(r, { status: value }, direction)
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
      <button type="button" onClick={() => onCellClick(r, c)} className="block w-full px-2 py-1.5 text-left">
        <span
          className={cn(
            "rounded-full px-2 py-0.5 text-xs font-medium",
            row.status === "incomplete" && "bg-amber-100 text-amber-800",
            row.status === "complete" && "bg-emerald-100 text-emerald-800",
            row.status === "cancelled" && "bg-red-100 text-red-800",
          )}
        >
          {STATUS_LABELS[row.status]}
        </span>
      </button>
    )
  }

  return (
    <select
      ref={selectRef}
      value={value}
      onChange={(e) => {
        setValue(e.target.value as IssuanceStatus)
        savedRef.current = false
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
        }
        // ArrowUp/Down are left to the native select so they change the status value;
        // Tab/Enter are what commit and leave this (last) column.
      }}
      onBlur={() => commit(null)}
      className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
    >
      {STATUS_OPTIONS.map((status) => (
        <option key={status} value={status}>
          {STATUS_LABELS[status]}
        </option>
      ))}
    </select>
  )
}
