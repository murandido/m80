"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Search, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import { User } from "@/schemas/user"

interface UserComboboxProps {
  users: User[]
  value: string
  onChange: (id: string) => void
  placeholder?: string
  disabled?: boolean
}

function matches(user: User, query: string): boolean {
  const needle = query.trim().toLowerCase()
  if (!needle) return true

  return (
    user.name.toLowerCase().includes(needle) ||
    user.phone.toLowerCase().includes(needle)
  )
}

export function UserCombobox({
  users,
  value,
  onChange,
  placeholder = "Buscar por nome...",
  disabled,
}: UserComboboxProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")

  const selected = users.find((u) => u.id === value)
  const filtered = useMemo(
    () => users.filter((u) => matches(u, query)),
    [users, query],
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
            {selected ? selected.name : placeholder}
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
            placeholder="Nome"
            className="pl-8"
          />
        </div>
        <div className="max-h-56 overflow-y-auto">
          {filtered.length === 0 && (
            <p className="px-2 py-3 text-center text-sm text-muted-foreground">
              Nenhum usuário encontrado.
            </p>
          )}
          {filtered.map((user) => (
            <button
              key={user.id}
              type="button"
              onClick={() => handleSelect(user.id)}
              className="flex w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent"
            >
              <span className="truncate">{user.name}</span>
              {user.id === value && <Check className="h-4 w-4 shrink-0" />}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}
