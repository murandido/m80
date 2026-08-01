"use client"

import { useCallback, useSyncExternalStore } from "react"

// Whether the issuances grid fetches from Traveos on its own as soon as a row has
// the minimum fields the airline needs.
//
// localStorage and not a column: this is an operator preference, not a fact about
// the business, and it is per person and per machine - the operator entering a
// batch of bookings wants it on, the one fixing yesterday's numbers does not.
const STORAGE_KEY = "issuances:autocomplete"

// The toggle lives in the filter bar and the behaviour lives in the grid, which
// are siblings under a Server Component - there is no shared parent to hold the
// state. A custom event is what keeps the two in step; `storage` alone would not,
// because the browser only fires it for *other* tabs.
const CHANGE_EVENT = "issuances:autocomplete-changed"

function subscribe(onChange: () => void): () => void {
  window.addEventListener(CHANGE_EVENT, onChange)
  window.addEventListener("storage", onChange)
  return () => {
    window.removeEventListener(CHANGE_EVENT, onChange)
    window.removeEventListener("storage", onChange)
  }
}

function readStored(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "on"
  } catch {
    // Private-mode Safari and a few enterprise policies throw here. Off is the
    // safe answer: the manual button still works, and nothing fires unasked.
    return false
  }
}

export function useAutocompletePreference(): [boolean, (enabled: boolean) => void] {
  // Off on the server, so the markup React hydrates with matches the default and
  // the real value lands on the first client render.
  const enabled = useSyncExternalStore(subscribe, readStored, () => false)

  const setEnabled = useCallback((next: boolean) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, next ? "on" : "off")
    } catch {
      // Ignored for the same reason as above - the toggle still works for this
      // page's lifetime, it just will not be remembered.
    }
    window.dispatchEvent(new Event(CHANGE_EVENT))
  }, [])

  return [enabled, setEnabled]
}
