"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Search } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useComboboxNavigation } from "@/hooks/use-combobox-navigation"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"
import { Agency } from "@/schemas/agency"

interface AgencyMultiComboboxProps {
  agencies: Agency[]
  // An empty array means "every agency", not "no results" - see the suppliers page, which
  // encodes that choice explicitly so it survives the round trip through the URL.
  value: string[]
  onChange: (ids: string[]) => void
  disabled?: boolean
  // What an empty selection reads as. The suppliers filter means "every agency" by it; a
  // user's grants mean the opposite - no agency at all - so the caller says which.
  emptyLabel?: string
}

function matches(agency: Agency, query: string): boolean {
  const needle = normalizeSearch(query)
  if (!needle) return true

  return (
    normalizeSearch(agency.name).includes(needle) ||
    agency.nicknames.some((n) => normalizeSearch(n.nickname).includes(needle))
  )
}

// Deliberately a sibling of AgencyCombobox rather than a `multiple` prop on it: the single
// version is used on eight screens where one agency is mandatory, and threading an
// either-string-or-array value through it would make every one of those call sites carry a
// case it never hits.
export function AgencyMultiCombobox({
  agencies,
  value,
  onChange,
  disabled,
  emptyLabel = "Todas as agências",
}: AgencyMultiComboboxProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")

  const filtered = useMemo(
    () => agencies.filter((a) => matches(a, query)),
    [agencies, query],
  )

  function toggle(id: string) {
    onChange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id])
    // The popover stays open on purpose: picking several agencies is the reason this
    // component exists, and closing after each one would make that four clicks apiece.
  }

  // Enter toggles and leaves the popover open, same as clicking: the hook only reports
  // which index was accepted, so multi-select needs nothing special here.
  const { activeIndex, listRef, handleKeyDown } = useComboboxNavigation({
    itemCount: filtered.length,
    resetKey: open ? query : null,
    onSelect: (index) => {
      const option = filtered[index]
      if (option) toggle(option.id)
    },
  })

  const label =
    value.length === 0
      ? emptyLabel
      : value.length === 1
        ? (agencies.find((a) => a.id === value[0])?.name ?? "1 agência")
        : `${value.length} agências`

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
          <span className={cn("min-w-0 truncate", value.length === 0 && "text-muted-foreground")}>
            {label}
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0 opacity-50" />
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
          {filtered.map((agency, index) => {
            const checked = value.includes(agency.id)
            return (
              <button
                key={agency.id}
                type="button"
                role="option"
                aria-selected={checked}
                onClick={() => toggle(agency.id)}
                className={cn(
                  "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent",
                  index === activeIndex && "bg-slate-200",
                )}
              >
                <span
                  className={cn(
                    "flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border",
                    checked ? "border-slate-800 bg-slate-800 text-slate-50" : "border-slate-300",
                  )}
                >
                  {checked && <Check className="h-3 w-3" />}
                </span>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">{agency.name}</span>
                  {agency.is_default && (
                    <span className="truncate text-xs text-muted-foreground">
                      Agência principal
                    </span>
                  )}
                </span>
              </button>
            )
          })}
        </div>
        {value.length > 0 && (
          <button
            type="button"
            onClick={() => onChange([])}
            className="mt-2 w-full rounded-sm px-2 py-1.5 text-left text-sm text-slate-500 hover:bg-accent"
          >
            {/* Says what an empty selection means here, which is the caller's call - see emptyLabel. */}
            Limpar seleção ({emptyLabel.toLowerCase()})
          </button>
        )}
      </PopoverContent>
    </Popover>
  )
}
