"use client"

import { useEffect, useId, useRef, useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { Search, X } from "lucide-react"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import { clearStickyFilters, writeStickyFilters } from "@/lib/sticky-filters"

interface ListSearchBarProps {
  /** Names the fields the search covers, so nobody has to guess what it matches. */
  placeholder: string
  /** Visible label; the placeholder alone disappears the moment you start typing. */
  label?: string
}

export function ListSearchBar({
  placeholder,
  label = "Buscar",
}: ListSearchBarProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const inputId = useId()
  const urlQuery = searchParams.get("q") ?? ""
  const [value, setValue] = useState(urlQuery)

  // The `q` this component last navigated to, or null when nothing it wrote is
  // still travelling. It exists because router.replace into a Server Component
  // only lands when the RSC payload comes back, so without it the box cannot tell
  // "the URL changed because someone else changed it" from "the URL is echoing
  // what I typed several keystrokes ago" - and it would obey the echo.
  const pendingWrite = useRef<string | null>(null)

  // Re-seed when `q` changes from outside this component - which is what happens
  // when useStickyFilters restores a remembered filter, since that lands after
  // mount. Without it the list comes back filtered while the box reads empty, and
  // the debounce below then wipes the filter the operator never saw.
  const [lastUrlQuery, setLastUrlQuery] = useState(urlQuery)
  if (urlQuery !== lastUrlQuery) {
    setLastUrlQuery(urlQuery)
    if (pendingWrite.current === null) {
      setValue(urlQuery)
    } else if (urlQuery === pendingWrite.current) {
      // Our own write arrived; `value` already says this. Anything typed since is
      // newer than the URL and the effect below will carry it over.
      pendingWrite.current = null
    }
    // Otherwise this is a stale echo of a write the operator has already typed
    // past - deleting "joao" one character at a time makes several of them - and
    // obeying it would put the deleted characters back and, worse, leave `value`
    // matching the URL so the real deletion is never written at all.
  }

  // Debounce URL updates so we don't refetch on every keystroke. Resetting to
  // page 1 keeps the result window valid after the filter changes.
  useEffect(() => {
    // Compared trimmed because it is written trimmed. Comparing the raw value
    // against a trimmed URL never converges for a lone space, and re-arms this
    // timeout forever.
    const desired = value.trim()
    const current = searchParams.get("q") ?? ""
    if (desired === current) return

    const timeout = setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString())
      if (desired) {
        params.set("q", desired)
      } else {
        params.delete("q")
      }
      params.delete("page")

      // Keep the memory in step with the navigation instead of letting
      // useStickyFilters observe the new URL against a cookie one step behind.
      const query = params.toString()
      if (query) {
        writeStickyFilters(pathname, query)
      } else {
        clearStickyFilters(pathname)
      }

      pendingWrite.current = desired
      router.replace(`${pathname}?${query}`)
    }, 350)

    return () => clearTimeout(timeout)
  }, [value, pathname, router, searchParams])

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={inputId} className="text-xs text-muted-foreground">
        {label}
      </Label>
      <InputGroup>
        <InputGroupInput
          id={inputId}
          // Not type="search": Chrome renders its own clear button for that, and
          // we already provide one — two X icons in the same field.
          type="text"
          value={value}
          onChange={(event) => setValue(event.target.value)}
          placeholder={placeholder}
        />
        <InputGroupAddon align="inline-end">
          {value ? (
            <button
              type="button"
              onClick={() => setValue("")}
              aria-label="Limpar busca"
              className="rounded-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <X className="h-4 w-4" />
            </button>
          ) : (
            <Search aria-hidden className="h-4 w-4 text-muted-foreground" />
          )}
        </InputGroupAddon>
      </InputGroup>
    </div>
  )
}
