"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Search, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import { Airline } from "@/schemas/airline"

interface AirlineComboboxProps {
  airlines: Airline[]
  value: string
  onChange: (id: string) => void
  placeholder?: string
  disabled?: boolean
}

function matches(airline: Airline, query: string): boolean {
  const needle = query.trim().toLowerCase()
  if (!needle) return true

  return (
    airline.name.toLowerCase().includes(needle) ||
    airline.code.toLowerCase().includes(needle)
  )
}

export function AirlineCombobox({
  airlines,
  value,
  onChange,
  placeholder = "Buscar por nome ou código...",
  disabled,
}: AirlineComboboxProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")

  const selected = airlines.find((a) => a.id === value)
  const filtered = useMemo(
    () => airlines.filter((a) => matches(a, query)),
    [airlines, query],
  )

  function handleSelect(id: string) {
    onChange(id === value ? "" : id)
    setOpen(false)
    setQuery("")
  }

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
          <span className={cn("truncate", !selected && "text-muted-foreground")}>
            {selected ? `${selected.name} (${selected.code})` : placeholder}
          </span>
          <div className="flex shrink-0 items-center gap-1">
            {selected && (
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
            placeholder="Nome ou código"
            className="pl-8"
          />
        </div>
        <div className="max-h-56 overflow-y-auto">
          {filtered.length === 0 && (
            <p className="px-2 py-3 text-center text-sm text-muted-foreground">
              Nenhuma companhia aérea encontrada.
            </p>
          )}
          {filtered.map((airline) => (
            <button
              key={airline.id}
              type="button"
              onClick={() => handleSelect(airline.id)}
              className="flex w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent"
            >
              <span className="flex flex-col">
                <span>{airline.name}</span>
                <span className="text-xs text-muted-foreground">{airline.code}</span>
              </span>
              {airline.id === value && <Check className="h-4 w-4 shrink-0" />}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}
