"use client"

import { useEffect, useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { Search, X } from "lucide-react"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"

export function AirportSearchBar() {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const [value, setValue] = useState(searchParams.get("q") ?? "")

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
    <InputGroup className="bg-slate-200">
      <InputGroupInput
        type="text"
        value={value}
        onChange={(event) => setValue(event.target.value)}
        placeholder="Buscar por código IATA, nome, cidade, estado ou país"
      />
      <InputGroupAddon align="inline-end">
        {value ? (
          <button
            type="button"
            onClick={() => setValue("")}
            aria-label="Limpar busca"
            className="text-slate-500 transition-colors hover:text-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        ) : (
          <Search className="text-foreground h-4 w-4" />
        )}
      </InputGroupAddon>
    </InputGroup>
  )
}
