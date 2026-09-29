"use client"

import { useMemo, useState } from "react"
import { Check, ListFilter, Search } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { NO_CLIENT } from "@/app/dashboard/sales/period-params"
import { useComboboxNavigation } from "@/hooks/use-combobox-navigation"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"
import type { Client } from "@/schemas/client"

interface FilterTriggerProps {
  active: boolean
  // Set on the narrow layout, which has no column header to hang an icon on: the filter
  // becomes a labelled button above the cards instead.
  label?: string
  name: string
}

function FilterTrigger({ active, label, name, ...props }: FilterTriggerProps) {
  if (label) {
    return (
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={cn("font-normal", active && "border-slate-400 bg-slate-100")}
        {...props}
      >
        <ListFilter className="h-4 w-4" />
        {label}
      </Button>
    )
  }
  return (
    <button
      type="button"
      aria-label={`Filtrar por ${name}`}
      className={cn(
        "inline-flex h-5 w-5 items-center justify-center rounded-sm transition-colors",
        active ? "bg-slate-300 text-slate-700" : "text-slate-300 hover:text-slate-500",
      )}
      {...props}
    >
      <ListFilter className="h-3.5 w-3.5" />
    </button>
  )
}

interface ClientColumnFilterProps {
  clients: Client[]
  // The chosen client ids, NO_CLIENT among them for "Sem cliente registrado". Empty means
  // every client.
  value: string[]
  onChange: (clientIds: string[]) => void
  label?: string
}

function matchesClient(client: Client, query: string): boolean {
  const needle = normalizeSearch(query)
  if (!needle) return true
  return (
    normalizeSearch(client.name).includes(needle) ||
    (client.phone ? normalizeSearch(client.phone).includes(needle) : false)
  )
}

const NO_CLIENT_LABEL = "Sem cliente registrado"

// Several clients at once, plus the sales nobody was linked to - the pile an operator goes
// through to link them. The popover stays open while picking, like AgencyMultiCombobox:
// closing after every tick would make choosing three clients three trips.
export function ClientColumnFilter({ clients, value, onChange, label }: ClientColumnFilterProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")

  // "Sem cliente registrado" sits first and is searchable by its own words, so typing
  // "sem" finds it the same way a name is found.
  const options = useMemo(() => {
    const noClient = matchesClient({ name: NO_CLIENT_LABEL } as Client, query)
      ? [{ id: NO_CLIENT, name: NO_CLIENT_LABEL, phone: null as string | null }]
      : []
    return [
      ...noClient,
      ...clients
        .filter((c) => matchesClient(c, query))
        .map((c) => ({ id: c.id, name: c.name, phone: c.phone ?? null })),
    ]
  }, [clients, query])

  function toggle(id: string) {
    onChange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id])
  }

  const { activeIndex, listRef, handleKeyDown } = useComboboxNavigation({
    itemCount: options.length,
    resetKey: open ? query : null,
    onSelect: (index) => {
      const option = options[index]
      if (option) toggle(option.id)
    },
  })

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setQuery("")
      }}
    >
      <PopoverTrigger asChild>
        <FilterTrigger active={value.length > 0} label={label} name="cliente" />
      </PopoverTrigger>
      <PopoverContent className="w-72 p-2" align="center">
        <div className="relative mb-2">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2" />
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Nome ou telefone"
            className="pl-8"
          />
        </div>
        <div ref={listRef} className="max-h-64 overflow-y-auto">
          {options.length === 0 && (
            <p className="text-muted-foreground px-2 py-3 text-center text-sm">
              Nenhum cliente encontrado.
            </p>
          )}
          {options.map((option, index) => {
            const checked = value.includes(option.id)
            return (
              <button
                key={option.id}
                type="button"
                role="option"
                aria-selected={checked}
                onClick={() => toggle(option.id)}
                className={cn(
                  "hover:bg-accent flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm",
                  index === activeIndex && "bg-slate-200",
                  option.id === NO_CLIENT && "italic",
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
                  <span className="truncate">{option.name}</span>
                  {option.phone && (
                    <span className="text-muted-foreground truncate text-xs">{option.phone}</span>
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
            className="hover:bg-accent mt-2 w-full rounded-sm px-2 py-1.5 text-left text-sm text-slate-500"
          >
            Limpar seleção (todos os clientes)
          </button>
        )}
      </PopoverContent>
    </Popover>
  )
}

// "paid" as the backend reads it: sales.paid, which is false for a partly paid sale too -
// "A Pagar" here means everything that still has money to come in.
const STATUS_OPTIONS = [
  { value: "", label: "Todas" },
  { value: "true", label: "Pago" },
  { value: "false", label: "A Pagar" },
] as const

interface StatusColumnFilterProps {
  value: string
  onChange: (paid: string) => void
  label?: string
}

export function StatusColumnFilter({ value, onChange, label }: StatusColumnFilterProps) {
  const [open, setOpen] = useState(false)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <FilterTrigger active={value !== ""} label={label} name="situação" />
      </PopoverTrigger>
      <PopoverContent className="w-44 p-1" align="center">
        {STATUS_OPTIONS.map((option) => (
          <button
            key={option.value || "all"}
            type="button"
            onClick={() => {
              onChange(option.value)
              setOpen(false)
            }}
            className="hover:bg-accent flex w-full items-center justify-between rounded-sm px-2 py-1.5 text-sm"
          >
            {option.label}
            {value === option.value && <Check className="h-4 w-4" />}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  )
}
