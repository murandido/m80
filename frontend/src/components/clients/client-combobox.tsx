"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Plus, Search, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useComboboxNavigation } from "@/hooks/use-combobox-navigation"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"
import { Client } from "@/schemas/client"

interface ClientComboboxProps {
  clients: Client[]
  value: string
  onChange: (id: string) => void
  placeholder?: string
  disabled?: boolean
  className?: string
  // When set, a search matching no existing client offers to create it, mirroring the
  // account column of the issuances grid (components/issuances/cells/combobox-cell.tsx).
  // The filter bars omit it: creating a client there would filter the list down to a
  // client that by definition has no sales yet.
  onCreate?: (name: string) => void
  // Reports the list opening and closing, for a table that highlights the row being edited.
  onOpenChange?: (open: boolean) => void
}

function matches(client: Client, query: string): boolean {
  const needle = normalizeSearch(query)
  if (!needle) return true

  return (
    normalizeSearch(client.name).includes(needle) ||
    (client.phone ? normalizeSearch(client.phone).includes(needle) : false)
  )
}

export function ClientCombobox({
  clients,
  value,
  onChange,
  placeholder = "Vincular cliente...",
  disabled,
  className,
  onCreate,
  onOpenChange,
}: ClientComboboxProps) {
  const [open, setOpenState] = useState(false)

  function setOpen(next: boolean) {
    setOpenState(next)
    onOpenChange?.(next)
  }
  const [query, setQuery] = useState("")

  const selected = clients.find((c) => c.id === value)
  const filtered = useMemo(
    () => clients.filter((c) => matches(c, query)),
    [clients, query],
  )

  // Offered only when nothing already carries that name, compared without accents: the
  // operator who types "joao silva" for an existing "João Silva" must be handed the
  // existing client, not an invitation to register a second spelling of the same person.
  const typed = query.trim()
  const showCreate =
    onCreate !== undefined &&
    typed !== "" &&
    !clients.some((c) => normalizeSearch(c.name) === normalizeSearch(typed))

  function handleSelect(id: string) {
    onChange(id === value ? "" : id)
    setOpen(false)
    setQuery("")
  }

  function handleCreate() {
    onCreate?.(typed)
    setOpen(false)
    setQuery("")
  }

  // The create entry sits after the matches, so the last navigable index is it - which is
  // what lets Enter register a client whose name matched nobody.
  const { activeIndex, listRef, handleKeyDown } = useComboboxNavigation({
    itemCount: filtered.length + (showCreate ? 1 : 0),
    resetKey: open ? query : null,
    onSelect: (index) => {
      const client = filtered[index]
      if (client) handleSelect(client.id)
      else if (showCreate) handleCreate()
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
          className={cn(
            "w-full justify-between bg-slate-200 font-normal hover:bg-slate-200",
            className,
          )}
        >
          <span className={cn("min-w-0 truncate", !selected && "text-muted-foreground")}>
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
            onKeyDown={handleKeyDown}
            placeholder="Nome ou telefone"
            className="pl-8"
          />
        </div>
        <div ref={listRef} className="max-h-56 overflow-y-auto">
          {filtered.length === 0 && !showCreate && (
            <p className="px-2 py-3 text-center text-sm text-muted-foreground">
              Nenhum cliente encontrado.
            </p>
          )}
          {filtered.map((client, index) => (
            <button
              key={client.id}
              type="button"
              onClick={() => handleSelect(client.id)}
              className={cn(
                "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent",
                index === activeIndex && "bg-slate-200",
              )}
            >
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate">{client.name}</span>
                {client.phone && (
                  <span className="truncate text-xs text-muted-foreground">
                    {client.phone}
                  </span>
                )}
              </span>
              {client.id === value && <Check className="h-4 w-4 shrink-0" />}
            </button>
          ))}

          {showCreate && (
            <button
              type="button"
              onClick={handleCreate}
              className={cn(
                "flex w-full items-start gap-1.5 rounded-sm px-2 py-2 text-left hover:bg-accent",
                filtered.length > 0 && "mt-1 border-t",
                activeIndex === filtered.length && "bg-slate-200",
              )}
            >
              <Plus className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span className="min-w-0">
                <span className="block truncate text-sm">
                  Criar cliente &quot;{typed}&quot;
                </span>
                <span className="block text-xs text-muted-foreground">
                  cadastra sem telefone e já vincula à venda
                </span>
              </span>
            </button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
