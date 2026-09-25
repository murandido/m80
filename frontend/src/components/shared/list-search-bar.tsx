"use client"

import { useEffect, useId, useState } from "react"
import { usePathname } from "next/navigation"
import { Search, X } from "lucide-react"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import { clearStickyFilters, writeStickyFilters } from "@/lib/sticky-filters"
import { useUrlQuery } from "@/hooks/use-url-query"

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
  const pathname = usePathname()
  // The query the screen is heading to, not the one that last landed: another control
  // may have written a filter the server has not answered yet, and building on the
  // landed URL would erase it.
  const { params: urlParams, update } = useUrlQuery()
  const inputId = useId()
  const urlQuery = urlParams.get("q") ?? ""
  const [value, setValue] = useState(urlQuery)

  // Re-seed when `q` changes from outside this component - which is what happens
  // when useStickyFilters restores a remembered filter, since that lands after
  // mount, and when "Limpar filtros" empties the URL. Without it the list comes back
  // filtered while the box reads empty, and the debounce below then wipes the filter
  // the operator never saw.
  //
  // `q` comes from useUrlQuery, which reports the query last asked for rather than
  // the one last landed, so it never steps back through the operator's own older
  // writes - deleting "joao" one character at a time used to echo each of them back
  // and retype them. What is left to skip is our own write arriving, which matches
  // what is typed up to the trimming it was written with.
  const [lastUrlQuery, setLastUrlQuery] = useState(urlQuery)
  if (urlQuery !== lastUrlQuery) {
    setLastUrlQuery(urlQuery)
    if (urlQuery !== value.trim()) setValue(urlQuery)
  }

  // Debounce URL updates so we don't refetch on every keystroke. Resetting to
  // page 1 keeps the result window valid after the filter changes.
  useEffect(() => {
    // Compared trimmed because it is written trimmed. Comparing the raw value
    // against a trimmed URL never converges for a lone space, and re-arms this
    // timeout forever.
    const desired = value.trim()
    if (desired === urlQuery) return

    const timeout = setTimeout(() => {
      update((params) => {
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
      })
    }, 350)

    return () => clearTimeout(timeout)
  }, [value, pathname, update, urlQuery])

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
