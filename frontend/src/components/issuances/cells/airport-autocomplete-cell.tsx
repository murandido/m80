"use client"

import { memo, useEffect, useRef, useState } from "react"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"
import { CellStateProps, useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"
import { Airport } from "@/schemas/airport"
import { DropdownPortal } from "@/components/issuances/cells/dropdown-portal"

interface AirportAutocompleteCellProps extends CellStateProps {
  r: number
  c: number
  field: "origin_iata_code" | "destination_iata_code"
}

// The dropdown never renders the whole seed (thousands of airports): the list is
// filtered in memory and capped, which is enough to reach any airport by typing a
// few characters while keeping the DOM small.
const MAX_RESULTS = 50

function airportLabel(airport: Airport): string {
  return `${airport.iata_code} - ${airport.city ?? airport.name}`
}

// An empty query yields the whole list (capped), in IATA order: opening the cell
// shows what is available instead of making the operator guess a prefix. A query
// matches across code/name/city/state/country, ranking IATA-prefix hits first to
// mirror the backend's priority.
function filterAirports(airports: Airport[], typed: string): Airport[] {
  const q = normalizeSearch(typed)
  if (q === "") return airports.slice(0, MAX_RESULTS)
  return airports
    .filter((a) =>
      [a.iata_code, a.name, a.city, a.state, a.country].some(
        (value) => (value ? normalizeSearch(value).includes(q) : false),
      ),
    )
    .sort((a, b) => {
      const aStarts = normalizeSearch(a.iata_code).startsWith(q)
      const bStarts = normalizeSearch(b.iata_code).startsWith(q)
      if (aStarts === bStarts) return 0
      return aStarts ? -1 : 1
    })
    .slice(0, MAX_RESULTS)
}

export const AirportAutocompleteCell = memo(function AirportAutocompleteCell({
  r,
  c,
  field,
  row,
  isEditing,
  pendingChar,
}: AirportAutocompleteCellProps) {
  const { onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave, airports } = useGridContext()
  const isThisCellEditing = isEditing
  const displayValue = row[field] ?? ""

  const [text, setText] = useState(displayValue)
  const [filtered, setFiltered] = useState<Airport[]>([])
  const [activeIndex, setActiveIndex] = useState(0)
  // Open state is tracked explicitly rather than derived from filtered.length,
  // because the list stays populated even when the query is empty - there is no
  // length that means "closed". Escape and picking an option are what close it.
  const [isOpen, setIsOpen] = useState(false)
  // Whether the user typed or arrowed since entering the cell. Enter/Tab only
  // accept the highlighted option once this is true; otherwise they commit and
  // move, so tabbing through an untouched cell never writes a silently-highlighted
  // airport.
  const [hasInteracted, setHasInteracted] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const savedRef = useRef(false)
  const acOpen = isOpen && filtered.length > 0

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ?? displayValue
      const startFiltered = filterAirports(airports, pendingChar ?? "")
      setText(startValue)
      setFiltered(startFiltered)
      // Highlight the row's current airport so accepting without moving is a no-op.
      const selectedIndex = startFiltered.findIndex((a) => a.iata_code === displayValue)
      setActiveIndex(selectedIndex === -1 ? 0 : selectedIndex)
      setIsOpen(true)
      setHasInteracted(false)
      savedRef.current = false
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.setSelectionRange(startValue.length, startValue.length)
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing])

  // The airport table now arrives from a browser fetch instead of with the page, so a
  // cell opened in the moment before it lands would seed an empty dropdown and stay that
  // way until the operator typed. Re-seed when the list arrives; the length guard keeps
  // this from overwriting a list the operator has already narrowed.
  useEffect(() => {
    if (!isThisCellEditing || filtered.length > 0) return
    setFiltered(filterAirports(airports, text))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [airports])

  // Keeps the highlighted option visible while arrowing through a list taller than the box.
  useEffect(() => {
    if (!acOpen) return
    listRef.current?.children[activeIndex]?.scrollIntoView({ block: "nearest" })
  }, [activeIndex, acOpen])

  function commitCode(code: string, direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true

    const normalized = code.trim().toUpperCase()
    if (normalized === "") {
      onSaveAndMove(r, {}, direction)
      return
    }
    onSaveAndMove(r, { [field]: normalized } as Partial<IssuanceRow>, direction)
  }

  useEffect(() => {
    if (isThisCellEditing) {
      registerActiveCellSave(() => commitCode(text, null))
      return () => registerActiveCellSave(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing, text])

  function handleChange(value: string) {
    setText(value)
    setFiltered(filterAirports(airports, value))
    setActiveIndex(0)
    setIsOpen(true)
    setHasInteracted(true)
  }

  // Accepting via keyboard keeps the cell in edit mode (dropdown closes): a second
  // Enter/Tab is what commits and moves.
  function acceptActive() {
    const active = filtered[activeIndex]
    if (!active) return
    setText(active.iata_code)
    setIsOpen(false)
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
            if (acOpen) setIsOpen(false)
            else onExitEdit()
          } else if (e.key === "ArrowDown") {
            e.preventDefault()
            if (acOpen) {
              setActiveIndex((i) => Math.min(i + 1, filtered.length - 1))
              setHasInteracted(true)
            } else commitCode(text, "down")
          } else if (e.key === "ArrowUp") {
            e.preventDefault()
            if (acOpen) {
              setActiveIndex((i) => Math.max(i - 1, 0))
              setHasInteracted(true)
            } else commitCode(text, "up")
          } else if (e.key === "Enter") {
            e.preventDefault()
            if (acOpen && hasInteracted) acceptActive()
            else commitCode(text, e.shiftKey ? "up" : "down")
          } else if (e.key === "Tab") {
            e.preventDefault()
            if (acOpen && hasInteracted) acceptActive()
            else commitCode(text, e.shiftKey ? "tab-left" : "tab-right")
          }
        }}
        onBlur={() => commitCode(text, null)}
        className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
      />
      {acOpen && (
        <DropdownPortal anchorRef={inputRef}>
          <div ref={listRef}>
            {filtered.map((airport, index) => (
              <button
                key={airport.id}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => commitCode(airport.iata_code, null)}
                className={cn(
                  "block w-full truncate px-2 py-1.5 text-left text-sm hover:bg-accent",
                  index === activeIndex && "bg-accent",
                )}
              >
                {airportLabel(airport)}
              </button>
            ))}
          </div>
        </DropdownPortal>
      )}
    </div>
  )
})
