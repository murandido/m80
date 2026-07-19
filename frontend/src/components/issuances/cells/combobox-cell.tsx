"use client"

import { useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"
import { DropdownPortal } from "@/components/issuances/cells/dropdown-portal"

export interface ComboOption {
  // Identity of the option itself, not of the row field it writes. The account column
  // needs the distinction: several options can share one account_id (one per alias), so
  // keying on the written value would make every alias of an account resolve to the first.
  key: string
  label: string
  // What picking this option writes onto the row. The caller owns the mapping, which is
  // what lets a single option set more than one field.
  patch: Partial<IssuanceRow>
}

interface ComboboxCellProps {
  r: number
  c: number
  options: ComboOption[]
  selectedKey: string | null
}

function filterOptions(options: ComboOption[], typed: string): ComboOption[] {
  const q = typed.trim().toLowerCase()
  if (q === "") return []
  return options
    .filter((o) => o.label.toLowerCase().includes(q))
    .sort((a, b) => {
      const aStarts = a.label.toLowerCase().startsWith(q)
      const bStarts = b.label.toLowerCase().startsWith(q)
      if (aStarts === bStarts) return 0
      return aStarts ? -1 : 1
    })
}

export function ComboboxCell({ r, c, options, selectedKey }: ComboboxCellProps) {
  const { sel, editing, pendingChar, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } =
    useGridContext()
  const isThisCellEditing = editing && sel.r === r && sel.c === c

  const currentOption = options.find((o) => o.key === selectedKey)
  const displayValue = currentOption?.label ?? ""

  const [text, setText] = useState(displayValue)
  const [filtered, setFiltered] = useState<ComboOption[]>([])
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)
  const acOpen = filtered.length > 0

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ?? displayValue
      setText(startValue)
      setFiltered(pendingChar ? filterOptions(options, pendingChar) : [])
      setActiveIndex(0)
      savedRef.current = false
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.setSelectionRange(startValue.length, startValue.length)
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing])

  function findExactMatch(value: string): ComboOption | undefined {
    return options.find((o) => o.label.toLowerCase() === value.trim().toLowerCase())
  }

  function commit(direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true

    const match = findExactMatch(text)
    if (!match) {
      onSaveAndMove(r, {}, direction)
      return
    }
    onSaveAndMove(r, match.patch, direction)
  }

  useEffect(() => {
    if (isThisCellEditing) {
      registerActiveCellSave(() => commit(null))
      return () => registerActiveCellSave(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing, text])

  function acceptActiveSuggestion() {
    const active = filtered[activeIndex]
    if (!active) return
    setText(active.label)
    setFiltered([])
  }

  function handleChange(value: string) {
    setText(value)
    const next = filterOptions(options, value)
    setFiltered(next)
    setActiveIndex(0)
    if (findExactMatch(value)) setFiltered([])
  }

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
    <div className="relative">
      <input
        ref={inputRef}
        value={text}
        onChange={(e) => handleChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault()
            if (acOpen) setFiltered([])
            else onExitEdit()
          } else if (e.key === "ArrowDown") {
            e.preventDefault()
            if (acOpen) setActiveIndex((i) => Math.min(i + 1, filtered.length - 1))
            else commit("down")
          } else if (e.key === "ArrowUp") {
            e.preventDefault()
            if (acOpen) setActiveIndex((i) => Math.max(i - 1, 0))
            else commit("up")
          } else if (e.key === "Enter") {
            e.preventDefault()
            if (acOpen) acceptActiveSuggestion()
            else commit("tab-right")
          } else if (e.key === "Tab") {
            e.preventDefault()
            if (acOpen) acceptActiveSuggestion()
            else commit(e.shiftKey ? "tab-left" : "tab-right")
          }
        }}
        onBlur={() => commit(null)}
        className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
      />
      {acOpen && (
        <DropdownPortal anchorRef={inputRef}>
          {filtered.map((option, index) => (
            <button
              key={option.key}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                setText(option.label)
                setFiltered([])
              }}
              className={cn(
                "block w-full truncate px-2 py-1.5 text-left text-sm hover:bg-accent",
                index === activeIndex && "bg-accent",
              )}
            >
              {option.label}
            </button>
          ))}
        </DropdownPortal>
      )}
    </div>
  )
}
