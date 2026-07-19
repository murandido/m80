"use client"

import { useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"
import { searchAirportsAction } from "@/actions/airport"
import { Airport } from "@/schemas/airport"
import { DropdownPortal } from "@/components/issuances/cells/dropdown-portal"

interface AirportAutocompleteCellProps {
  r: number
  c: number
  field: "origin_iata_code" | "destination_iata_code"
}

function airportLabel(airport: Airport): string {
  return `${airport.iata_code} - ${airport.city ?? airport.name}`
}

export function AirportAutocompleteCell({ r, c, field }: AirportAutocompleteCellProps) {
  const { rows, sel, editing, pendingChar, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } =
    useGridContext()
  const row = rows[r]
  const isThisCellEditing = editing && sel.r === r && sel.c === c
  const displayValue = row[field] ?? ""

  const [text, setText] = useState(displayValue)
  const [results, setResults] = useState<Airport[]>([])
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const savedRef = useRef(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const acOpen = results.length > 0

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ?? displayValue
      setText(startValue)
      setResults([])
      setActiveIndex(0)
      savedRef.current = false
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.setSelectionRange(startValue.length, startValue.length)
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing])

  // TODO: align with the other combobox cells, which open showing every option and filter
  // as the user types. Airports cannot do that yet: they are searched on the backend per
  // keystroke and there is no endpoint that lists them, so an empty query has nothing to
  // show. Revisit once the full airport CRUD exists and a list endpoint is available.
  function scheduleSearch(query: string) {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (query.trim() === "") {
      setResults([])
      return
    }
    debounceRef.current = setTimeout(async () => {
      const result = await searchAirportsAction(query)
      if ("airports" in result) {
        setResults(result.airports)
        setActiveIndex(0)
      }
    }, 200)
  }

  function commit(direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true

    const code = text.trim().toUpperCase()
    if (code === "") {
      onSaveAndMove(r, {}, direction)
      return
    }
    onSaveAndMove(r, { [field]: code } as Partial<IssuanceRow>, direction)
  }

  useEffect(() => {
    if (isThisCellEditing) {
      registerActiveCellSave(() => commit(null))
      return () => registerActiveCellSave(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing, text])

  function acceptActive() {
    const active = results[activeIndex]
    if (!active) return
    setText(active.iata_code)
    setResults([])
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
        onChange={(e) => {
          const value = e.target.value
          setText(value)
          scheduleSearch(value)
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault()
            if (acOpen) setResults([])
            else onExitEdit()
          } else if (e.key === "ArrowDown") {
            e.preventDefault()
            if (acOpen) setActiveIndex((i) => Math.min(i + 1, results.length - 1))
            else commit("down")
          } else if (e.key === "ArrowUp") {
            e.preventDefault()
            if (acOpen) setActiveIndex((i) => Math.max(i - 1, 0))
            else commit("up")
          } else if (e.key === "Enter") {
            e.preventDefault()
            if (acOpen) acceptActive()
            else commit("tab-right")
          } else if (e.key === "Tab") {
            e.preventDefault()
            if (acOpen) acceptActive()
            else commit(e.shiftKey ? "tab-left" : "tab-right")
          }
        }}
        onBlur={() => commit(null)}
        className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
      />
      {acOpen && (
        <DropdownPortal anchorRef={inputRef}>
          {results.map((airport, index) => (
            <button
              key={airport.id}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                setText(airport.iata_code)
                setResults([])
              }}
              className={cn(
                "block w-full truncate px-2 py-1.5 text-left text-sm hover:bg-accent",
                index === activeIndex && "bg-accent",
              )}
            >
              {airportLabel(airport)}
            </button>
          ))}
        </DropdownPortal>
      )}
    </div>
  )
}
