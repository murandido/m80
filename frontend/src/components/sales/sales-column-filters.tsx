"use client"

import { useState } from "react"
import { Check, ListFilter } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { ClientCombobox } from "@/components/clients/client-combobox"
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
  value: string
  onChange: (clientId: string) => void
  label?: string
}

export function ClientColumnFilter({ clients, value, onChange, label }: ClientColumnFilterProps) {
  const [open, setOpen] = useState(false)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <FilterTrigger active={value !== ""} label={label} name="cliente" />
      </PopoverTrigger>
      <PopoverContent className="w-64 p-2" align="center">
        <ClientCombobox
          clients={clients}
          value={value}
          placeholder="Todos os clientes"
          onChange={(clientId) => {
            onChange(clientId)
            setOpen(false)
          }}
        />
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
