"use client"

import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type RefObject } from "react"

interface ComboboxNavigationOptions {
  // How many options the list is currently rendering, including a trailing "create"
  // entry when the caller offers one.
  itemCount: number
  // Anything that should send the highlight back to the top: the search term, and
  // whether the popover is open. A narrowed list must never keep highlighting the row
  // the previous term happened to put at that index.
  resetKey: unknown
  onSelect: (index: number) => void
}

interface ComboboxNavigation {
  activeIndex: number
  listRef: RefObject<HTMLDivElement | null>
  handleKeyDown: (event: KeyboardEvent<HTMLInputElement>) => void
}

// Arrow/Enter navigation for the panel's search-and-pick popovers, which are all built
// the same way: a search input plus a scrollable div whose direct children are the
// option buttons. Domain-agnostic on purpose - it only ever deals in indices, so the
// caller decides what selecting index N means (pick a client, toggle an agency, create
// a record).
//
// The highlight starts on the first option so that identifying the row and pressing
// Enter is the whole interaction; before this, picking anything meant reaching for the
// mouse. Mirrors the issuances grid cell (components/issuances/cells/combobox-cell.tsx),
// which is not refactored to use this: Enter there commits the cell and moves down, so
// it carries gating this does not need.
//
// Escape is deliberately absent - Radix's PopoverContent already closes on it.
export function useComboboxNavigation({
  itemCount,
  resetKey,
  onSelect,
}: ComboboxNavigationOptions): ComboboxNavigation {
  const [activeIndex, setActiveIndex] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  // Adjusted during render rather than in an effect: an effect would paint one frame with
  // the stale highlight before correcting it, and React flags the cascading render.
  const [lastResetKey, setLastResetKey] = useState(resetKey)
  if (resetKey !== lastResetKey) {
    setLastResetKey(resetKey)
    setActiveIndex(0)
  }

  // Keeps the highlighted option visible while arrowing through a list taller than the box.
  useEffect(() => {
    if (itemCount === 0) return
    listRef.current?.children[activeIndex]?.scrollIntoView({ block: "nearest" })
  }, [activeIndex, itemCount])

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      if (itemCount === 0) return

      if (event.key === "ArrowDown") {
        event.preventDefault()
        setActiveIndex((index) => Math.min(index + 1, itemCount - 1))
      } else if (event.key === "ArrowUp") {
        event.preventDefault()
        setActiveIndex((index) => Math.max(index - 1, 0))
      } else if (event.key === "Enter") {
        // Without this the key would reach the form the combobox sits in and submit it.
        event.preventDefault()
        onSelect(activeIndex)
      }
    },
    [activeIndex, itemCount, onSelect],
  )

  return { activeIndex, listRef, handleKeyDown }
}
