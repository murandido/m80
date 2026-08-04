"use client"

import { useEffect } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { readStickyFiltersFromDocument, writeStickyFilters } from "@/lib/sticky-filters"

// Keeps a screen's filters across navigation, from a single place: it remembers
// whatever the URL currently says, and puts it back when the operator returns to a
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

  useEffect(() => {
    const current = searchParams.toString()
    if (current !== "") {
      writeStickyFilters(pathname, current)
      return
    }

    // Bare URL: either the operator just arrived, or they cleared the filters -
    // and clearing deletes the cookie first, so there is nothing to put back.
    const remembered = readStickyFiltersFromDocument(pathname)
    if (remembered) router.replace(`${pathname}?${remembered}`)
  }, [pathname, router, searchParams])
}
