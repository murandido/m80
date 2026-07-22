"use client"

import { useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { Direction } from "@/lib/grid-navigation"
import { LoyaltyProgramBadge } from "@/components/accounts/loyalty-program-badge"
import { DropdownPortal } from "@/components/issuances/cells/dropdown-portal"

interface Option {
  value: string
  label: string
}

// An empty query yields the whole list, in its original order: opening the cell should
// show what is available instead of making the operator guess a prefix to type.
function filterByName(options: Option[], typed: string): Option[] {
  const q = typed.trim().toLowerCase()
  if (q === "") return options
  return options
    .filter((o) => o.label.toLowerCase().includes(q))
    .sort((a, b) => {
      const aStarts = a.label.toLowerCase().startsWith(q)
      const bStarts = b.label.toLowerCase().startsWith(q)
      if (aStarts === bStarts) return 0
      return aStarts ? -1 : 1
    })
}

export function CiaAereaCell({ r, c }: { r: number; c: number }) {
  const {
    rows,
    sel,
    editing,
    pendingChar,
    onCellClick,
    onSaveAndMove,
    onExitEdit,
    registerActiveCellSave,
    loyaltyPrograms,
    airlines,
  } = useGridContext()
  const row = rows[r]
  const isThisCellEditing = editing && sel.r === r && sel.c === c

  const currentProgram = loyaltyPrograms.find((lp) => lp.id === row.loyalty_program_id)
  const currentAirline = airlines.find((a) => a.id === row.airline_id)

  // The default airline is implied by the program, so it is left out of both the read
  // view and the edit text - showing it would make the user delete a redundant suffix.
  const showsNonDefaultAirline =
    currentProgram !== undefined &&
    currentAirline !== undefined &&
    currentAirline.id !== currentProgram.default_airline_id

  const displayText = currentProgram
    ? showsNonDefaultAirline
      ? `${currentProgram.name}/${currentAirline!.name}`
      : currentProgram.name
    : ""

  const programOptions: Option[] = loyaltyPrograms.map((lp) => ({ value: lp.id, label: lp.name }))
  const airlineOptions: Option[] = airlines.map((a) => ({ value: a.id, label: a.name }))

  const [text, setText] = useState(displayText)
  const [stage, setStage] = useState<"program" | "airline">("program")
  const [prefix, setPrefix] = useState("")
  const [filtered, setFiltered] = useState<Option[]>([])
  const [activeIndex, setActiveIndex] = useState(0)
  // Open state is tracked explicitly rather than derived from filtered.length, because the
  // list now stays populated even when the query is empty - there is no length that means
  // "closed" anymore. Escape and picking an option are what close it.
  const [isOpen, setIsOpen] = useState(false)
  // Whether the user typed or arrowed since entering the cell. Enter/Tab only accept the
  // highlighted option once this is true; otherwise they commit and move as they always
  // did. Without it, tabbing through an untouched cell would write whatever happens to be
  // highlighted - on an empty cell, that is the first program, silently invented.
  const [hasInteracted, setHasInteracted] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const savedRef = useRef(false)
  const acOpen = isOpen && filtered.length > 0

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ?? displayText
      setText(startValue)
      // Opening on a cell that already names a program still lists every program, so the
      // operator sees the alternatives; only a typed character narrows the list on open.
      const startFiltered = filterByName(programOptions, pendingChar ?? "")
      setStage("program")
      setPrefix("")
      setFiltered(startFiltered)
      // Highlight the row's current program so accepting without moving is a no-op.
      const selectedIndex = startFiltered.findIndex((o) => o.value === row.loyalty_program_id)
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

  // Keeps the highlighted option visible while arrowing through a list taller than the box.
  useEffect(() => {
    if (!acOpen) return
    listRef.current?.children[activeIndex]?.scrollIntoView({ block: "nearest" })
  }, [activeIndex, acOpen])

  function handleChange(value: string) {
    setText(value)
    const slashIndex = value.indexOf("/")
    if (slashIndex === -1) {
      setStage("program")
      setPrefix("")
      setFiltered(filterByName(programOptions, value))
    } else {
      const airlineTyped = value.slice(slashIndex + 1)
      setStage("airline")
      setPrefix(value.slice(0, slashIndex + 1))
      setFiltered(filterByName(airlineOptions, airlineTyped))
    }
    setActiveIndex(0)
    setIsOpen(true)
    setHasInteracted(true)
  }

  // Accepting keeps the cell in edit mode rather than committing: after picking a program
  // the operator may still type "/" to override the default airline.
  function acceptActive() {
    const active = filtered[activeIndex]
    if (!active) return
    setText(stage === "program" ? active.label : prefix + active.label)
    setIsOpen(false)
  }

  // An account belongs to one loyalty program, so it cannot carry over to a different
  // one. Clearing it (and its alias) leaves the cell empty, which is the prompt to pick
  // a valid account. Only on an actual program change - reselecting the same program, or
  // just swapping the airline, must not wipe the account the user already chose.
  function accountResetFor(programId: string): Partial<IssuanceRow> {
    // Nothing to clear, and nothing to warn about, when no account was chosen yet.
    if (programId === row.loyalty_program_id || row.account_id === null) return {}
    return { account_id: null, account_nickname_id: null }
  }

  function commit(direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true

    const slashIndex = text.indexOf("/")
    if (slashIndex === -1) {
      const program = loyaltyPrograms.find((lp) => lp.name.toLowerCase() === text.trim().toLowerCase())
      if (!program || text.trim() === "") {
        onSaveAndMove(r, {}, direction)
        return
      }
      onSaveAndMove(
        r,
        {
          loyalty_program_id: program.id,
          airline_id: program.default_airline_id,
          ...accountResetFor(program.id),
        },
        direction,
      )
      return
    }

    const programName = text.slice(0, slashIndex)
    const airlineName = text.slice(slashIndex + 1)
    const program = loyaltyPrograms.find((lp) => lp.name.toLowerCase() === programName.trim().toLowerCase())
    const airline = airlines.find((a) => a.name.toLowerCase() === airlineName.trim().toLowerCase())
    if (!program || !airline) {
      onSaveAndMove(r, {}, direction)
      return
    }
    onSaveAndMove(
      r,
      { loyalty_program_id: program.id, airline_id: airline.id, ...accountResetFor(program.id) },
      direction,
    )
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
        className="flex w-full items-center gap-1.5 truncate px-2 py-1.5 text-left"
      >
        {currentProgram ? (
          <>
            <LoyaltyProgramBadge loyaltyProgram={currentProgram} />
            {showsNonDefaultAirline && (
              <span className="truncate text-xs text-muted-foreground">({currentAirline!.name})</span>
            )}
          </>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </button>
    )
  }

  return (
    <div className="relative">
      <input
        ref={inputRef}
        value={text}
        placeholder="programa/cia"
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
            } else commit("down")
          } else if (e.key === "ArrowUp") {
            e.preventDefault()
            if (acOpen) {
              setActiveIndex((i) => Math.max(i - 1, 0))
              setHasInteracted(true)
            } else commit("up")
          } else if (e.key === "Enter") {
            e.preventDefault()
            if (acOpen && hasInteracted) acceptActive()
            else commit(e.shiftKey ? "up" : "down")
          } else if (e.key === "Tab") {
            e.preventDefault()
            if (acOpen && hasInteracted) acceptActive()
            else commit(e.shiftKey ? "tab-left" : "tab-right")
          }
        }}
        onBlur={() => commit(null)}
        className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
      />
      {acOpen && (
        <DropdownPortal anchorRef={inputRef}>
          <div ref={listRef}>
          {filtered.map((option, index) => {
            // Airlines carry no color of their own, so the badge is limited to the
            // program stage - the same dropdown serves both.
            const program =
              stage === "program" ? loyaltyPrograms.find((lp) => lp.id === option.value) : undefined

            return (
              <button
                key={option.value}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  setText(stage === "program" ? option.label : prefix + option.label)
                  setIsOpen(false)
                  inputRef.current?.focus()
                }}
                className={cn(
                  "flex w-full items-center px-2 py-1.5 text-left text-sm hover:bg-accent",
                  index === activeIndex && "bg-accent",
                )}
              >
                {program ? (
                  <LoyaltyProgramBadge loyaltyProgram={program} />
                ) : (
                  <span className="truncate">{option.label}</span>
                )}
              </button>
            )
          })}
          </div>
        </DropdownPortal>
      )}
    </div>
  )
}
