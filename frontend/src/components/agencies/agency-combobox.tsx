"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Search, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useComboboxNavigation } from "@/hooks/use-combobox-navigation"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"
import { Agency } from "@/schemas/agency"

interface AgencyComboboxProps {
  agencies: Agency[]
  value: string
  onChange: (id: string) => void
  placeholder?: string
  disabled?: boolean
  // Clearing is hidden where the caller always needs one agency selected (the sales
  // views, which fall back to the main agency server-side rather than showing everything).
  clearable?: boolean
}

function matches(agency: Agency, query: string): boolean {
  const needle = normalizeSearch(query)
  if (!needle) return true

  return (
    normalizeSearch(agency.name).includes(needle) ||
    agency.nicknames.some((n) => normalizeSearch(n.nickname).includes(needle))
  )
}

export function AgencyCombobox({
  agencies,
  value,
  onChange,
  placeholder = "Buscar por nome...",
  disabled,
  clearable = true,
}: AgencyComboboxProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")

  const selected = agencies.find((a) => a.id === value)
  const filtered = useMemo(
    () => agencies.filter((a) => matches(a, query)),
    [agencies, query],
  )

  function handleSelect(id: string) {
    onChange(id === value && clearable ? "" : id)
    setOpen(false)
    setQuery("")
  }

  const { activeIndex, listRef, handleKeyDown } = useComboboxNavigation({
    itemCount: filtered.length,
    resetKey: open ? query : null,
    onSelect: (index) => {
      const option = filtered[index]
      if (option) handleSelect(option.id)
    },
  })

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className="w-full justify-between bg-slate-200 font-normal hover:bg-slate-200"
        >
          <span className={cn("min-w-0 truncate", !selected && "text-muted-foreground")}>
            {selected ? selected.name : placeholder}
          </span>
          <div className="flex shrink-0 items-center gap-1">
            {selected && clearable && (
              <span
                role="button"
                tabIndex={-1}
                onClick={(e) => {
                  e.stopPropagation()
                  onChange("")
                }}
                className="rounded-full p-0.5 hover:bg-slate-300"
                aria-label="Limpar seleção"
              >
                <X className="h-3.5 w-3.5" />
              </span>
            )}
            <ChevronsUpDown className="h-4 w-4 opacity-50" />
          </div>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-2" align="start">
        <div className="relative mb-2">
          <Search className="pointer-events-none absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Nome ou apelido"
            className="pl-8"
          />
        </div>
        <div ref={listRef} className="max-h-56 overflow-y-auto">
          {filtered.length === 0 && (
            <p className="px-2 py-3 text-center text-sm text-muted-foreground">
              Nenhuma agência encontrada.
            </p>
          )}
          {filtered.map((agency, index) => (
            <button
              key={agency.id}
              type="button"
              onClick={() => handleSelect(agency.id)}
              className={cn(
                "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent",
                index === activeIndex && "bg-slate-200",
              )}
            >
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate">{agency.name}</span>
                {agency.is_default && (
                  <span className="truncate text-xs text-muted-foreground">
                    Agência principal
                  </span>
                )}
              </span>
              {agency.id === value && <Check className="h-4 w-4 shrink-0" />}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}
