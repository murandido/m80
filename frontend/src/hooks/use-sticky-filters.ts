"use client"

import { useEffect, useRef } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import {
  clearStickyFilters,
  readStickyFiltersFromDocument,
  writeStickyFilters,
} from "@/lib/sticky-filters"

// Keeps a screen's filters across navigation, from a single place: it remembers
// whatever the URL currently says, and puts it back when the operator arrives at a
// bare URL (every sidebar link points at the path with no query string).
//
// Restoring writes the filters back into the URL instead of only rendering with
// them. From there on the URL is authoritative again, which is what the controls
// assume when they merge or clear one dimension - a bar merging onto an empty URL
// would silently drop every other filter that was restored.
//
// The server reads the same cookie, so the restored screen is already correct on
// first paint; this replace only syncs the address bar.
export function useStickyFilters() {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  // Restoring happens once per arrival at a screen, not every time the URL goes
  // bare. Without this the hook cannot tell "the operator just got here" from "the
  // operator just emptied the filters", and it would put back what they cleared -
  // which is what made the search box retype itself the moment it was emptied.
  //
  // Keyed by pathname rather than a plain boolean so a hook instance that outlives
  // a route change still treats the new screen as a fresh arrival.
  const settledFor = useRef<string | null>(null)

  useEffect(() => {
    const current = searchParams.toString()

    if (current !== "") {
      settledFor.current = pathname
      writeStickyFilters(pathname, current)
      return
    }

    if (settledFor.current !== pathname) {
      settledFor.current = pathname
      const remembered = readStickyFiltersFromDocument(pathname)
      if (remembered) router.replace(`${pathname}?${remembered}`)
      return
    }

    // Bare URL after the operator was already here: they cleared the filters
    // themselves. Forget, so coming back to the screen does not resurrect them.
    clearStickyFilters(pathname)
  }, [pathname, router, searchParams])
}
