"use client"

import { useEffect, useRef, useState } from "react"
import { Loader2, Plus } from "lucide-react"
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
  // When set, text that matches no option gets a final "create it" entry in the dropdown.
  // Columns that omit this behave exactly as before.
  create?: {
    // What the entry says it will do, e.g. `Criar conta "João Silva"`.
    label: (typed: string) => string
    // The side effects spelled out, so picking it is never a surprise.
    hint: string
    // Non-null renders the entry as an inert explanation instead of an action.
    disabledReason: string | null
    onCreate: (typed: string) => void
  }
  // Shown greyed while `create` is still in flight, so the cell doesn't fall back to an
  // em dash and look like the typing was thrown away.
  pendingLabel?: string
}

// An empty query yields the whole list, in its original order: opening the cell should
// show what is available instead of making the operator guess a prefix to type.
function filterOptions(options: ComboOption[], typed: string): ComboOption[] {
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

export function ComboboxCell({
  r,
  c,
  options,
  selectedKey,
  create,
  pendingLabel,
}: ComboboxCellProps) {
  const { sel, editing, pendingChar, onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave } =
    useGridContext()
  const isThisCellEditing = editing && sel.r === r && sel.c === c

  const currentOption = options.find((o) => o.key === selectedKey)
  const displayValue = currentOption?.label ?? ""

  const [text, setText] = useState(displayValue)
  const [filtered, setFiltered] = useState<ComboOption[]>([])
  const [activeIndex, setActiveIndex] = useState(0)
  // Open state is tracked explicitly rather than derived from filtered.length, because the
  // list now stays populated even when the query is empty - there is no length that means
  // "closed" anymore. Escape and picking an option are what close it.
  const [isOpen, setIsOpen] = useState(false)
  // Whether the user typed or arrowed since entering the cell. Enter/Tab only accept the
  // highlighted option once this is true; otherwise they commit and move as they always
  // did. Without it, tabbing through an untouched cell would write whatever happens to be
  // highlighted - on an empty cell, that is the first option, silently invented.
  const [hasInteracted, setHasInteracted] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const savedRef = useRef(false)
  // The option accepted via keyboard, kept so the commit that follows saves that exact
  // option instead of re-deriving it from the label - see commitOption.
  const acceptedRef = useRef<ComboOption | null>(null)
  // Text the user chose to turn into a new record. Held until commit rather than acted on
  // at pick time, so leaving the cell with Escape creates nothing.
  const pendingCreateRef = useRef<string | null>(null)

  // The create entry is offered for text that matches nothing. It is still rendered when
  // disabled, because "you must pick an airline first" is more useful than an empty list.
  const showCreate =
    create !== undefined && text.trim() !== "" && findExactMatch(text) === undefined
  // A disabled create entry is displayed but not reachable by the arrow keys.
  const createIndex = filtered.length
  const createIsSelectable = showCreate && create.disabledReason === null
  const navigableCount = filtered.length + (createIsSelectable ? 1 : 0)
  const acOpen = isOpen && (filtered.length > 0 || showCreate)
  // Whether Enter/Tab have something to accept. False when the only thing on screen is a
  // disabled create hint - then those keys must commit and move, not swallow themselves.
  const hasAcceptTarget = filtered.length > 0 || createIsSelectable

  useEffect(() => {
    if (isThisCellEditing) {
      const startValue = pendingChar ?? displayValue
      // Entering a cell that already has a value still lists everything, so the operator
      // can see the alternatives; only a typed character narrows the list on open.
      const startFiltered = filterOptions(options, pendingChar ?? "")
      setText(startValue)
      setFiltered(startFiltered)
      // Highlight the row's current value so accepting without moving is a no-op.
      const selectedIndex = startFiltered.findIndex((o) => o.key === selectedKey)
      setActiveIndex(selectedIndex === -1 ? 0 : selectedIndex)
      setIsOpen(true)
      setHasInteracted(false)
      savedRef.current = false
      acceptedRef.current = null
      pendingCreateRef.current = null
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

  function findExactMatch(value: string): ComboOption | undefined {
    return options.find((o) => o.label.toLowerCase() === value.trim().toLowerCase())
  }

  function commit(direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true

    // Creation runs in the background and patches the row when it lands, so the cell
    // reports an empty patch and lets the operator move on immediately.
    const pendingCreate = pendingCreateRef.current
    if (pendingCreate !== null && create) {
      create.onCreate(pendingCreate)
      onSaveAndMove(r, {}, direction)
      return
    }

    const accepted = acceptedRef.current
    if (accepted && accepted.label === text.trim()) {
      onSaveAndMove(r, accepted.patch, direction)
      return
    }

    // Text untouched: keep the option the row already points at. Resolving it by label
    // could land on a different option that happens to share the label.
    if (text.trim() === displayValue.trim()) {
      onSaveAndMove(r, {}, direction)
      return
    }

    const match = findExactMatch(text)
    if (!match) {
      onSaveAndMove(r, {}, direction)
      return
    }
    onSaveAndMove(r, match.patch, direction)
  }

  // Commits a specific option rather than re-resolving the text. The account column builds
  // one option per nickname, so distinct options can share a label - findExactMatch would
  // resolve every one of them to the first, saving the wrong account.
  function commitOption(option: ComboOption, direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true
    onSaveAndMove(r, option.patch, direction)
  }

  useEffect(() => {
    if (isThisCellEditing) {
      registerActiveCellSave(() => commit(null))
      return () => registerActiveCellSave(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing, text])

  function acceptCreate() {
    pendingCreateRef.current = text.trim()
    acceptedRef.current = null
    setIsOpen(false)
  }

  function acceptActiveSuggestion() {
    if (createIsSelectable && activeIndex === createIndex) {
      acceptCreate()
      return
    }
    const active = filtered[activeIndex]
    if (!active) return
    setText(active.label)
    acceptedRef.current = active
    pendingCreateRef.current = null
    setIsOpen(false)
  }

  function handleChange(value: string) {
    setText(value)
    acceptedRef.current = null
    // Editing the text after choosing "create" cancels that intent - the name that would
    // be created is no longer the name on screen.
    pendingCreateRef.current = null
    setFiltered(filterOptions(options, value))
    setActiveIndex(0)
    setIsOpen(true)
    setHasInteracted(true)
  }

  if (!isThisCellEditing) {
    return (
      <button
        type="button"
        onClick={() => onCellClick(r, c)}
        className="block w-full truncate px-2 py-1.5 text-left"
      >
        {pendingLabel ? (
          <span className="flex items-center gap-1.5 text-muted-foreground">
            <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
            <span className="truncate">{pendingLabel}</span>
          </span>
        ) : (
          displayValue || <span className="text-muted-foreground">—</span>
        )}
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
              setActiveIndex((i) => Math.min(i + 1, navigableCount - 1))
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
            if (acOpen && hasInteracted && hasAcceptTarget) acceptActiveSuggestion()
            else commit(e.shiftKey ? "up" : "down")
          } else if (e.key === "Tab") {
            e.preventDefault()
            if (acOpen && hasInteracted && hasAcceptTarget) acceptActiveSuggestion()
            else commit(e.shiftKey ? "tab-left" : "tab-right")
          }
        }}
        onBlur={() => commit(null)}
        className="w-full bg-transparent px-2 py-1.5 outline-none ring-2 ring-primary"
      />
      {acOpen && (
        <DropdownPortal anchorRef={inputRef}>
          <div ref={listRef}>
            {filtered.map((option, index) => (
              <button
                key={option.key}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => commitOption(option, null)}
                className={cn(
                  "block w-full truncate px-2 py-1.5 text-left text-sm hover:bg-accent",
                  index === activeIndex && "bg-accent",
                )}
              >
                {option.label}
              </button>
            ))}

            {showCreate &&
              (create.disabledReason !== null ? (
                <p className="border-t px-2 py-2 text-xs text-muted-foreground">
                  {create.disabledReason}
                </p>
              ) : (
                <button
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={acceptCreate}
                  className={cn(
                    "flex w-full items-start gap-1.5 border-t px-2 py-2 text-left hover:bg-accent",
                    activeIndex === createIndex && "bg-accent",
                  )}
                >
                  <Plus className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span className="min-w-0">
                    <span className="block truncate text-sm">{create.label(text.trim())}</span>
                    <span className="block text-xs text-muted-foreground">{create.hint}</span>
                  </span>
                </button>
              ))}
          </div>
        </DropdownPortal>
      )}
    </div>
  )
}
