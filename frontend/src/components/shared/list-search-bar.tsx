"use client"

import { useEffect, useId, useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { Search, X } from "lucide-react"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"

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

  // Re-seed when the URL's q changes from outside this component - which is what
  // happens when useStickyFilters restores a remembered filter, since that lands
  // after mount. Without it the list comes back filtered while the box reads
  // empty, and the debounce below then wipes the filter the operator never saw.
  const [lastUrlQuery, setLastUrlQuery] = useState(urlQuery)
  if (urlQuery !== lastUrlQuery) {
    setLastUrlQuery(urlQuery)
    setValue(urlQuery)
  }

  // Debounce URL updates so we don't refetch on every keystroke. Resetting to
  // page 1 keeps the result window valid after the filter changes.
  useEffect(() => {
    const current = searchParams.get("q") ?? ""
    if (value === current) return

    const timeout = setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString())
      if (value.trim()) {
        params.set("q", value.trim())
      } else {
        params.delete("q")
      }
      params.delete("page")
      router.replace(`${pathname}?${params.toString()}`)
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
