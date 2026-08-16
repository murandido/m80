"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Plus, Search, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { useComboboxNavigation } from "@/hooks/use-combobox-navigation"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"
import { Account } from "@/schemas/account"
import { Person } from "@/schemas/person"

interface AccountComboboxProps {
  accounts: Account[]
  // Used to resolve an account's holder into a readable name, mirroring the grid.
  people: Person[]
  value: string
  onChange: (id: string) => void
  placeholder?: string
  disabled?: boolean
  // When set, a search matching no existing account offers to register it - the same
  // entry the issuances grid has in its Conta column, and for the same reason: the
  // operator has a name in front of them and no reason to leave the screen to use it.
  // Callers that only pick among existing accounts (the filter bars) leave it out.
  onCreate?: (name: string) => void
  // Seeds the search box each time the popover opens. Used where the caller already
  // knows the word being looked for - the spreadsheet importer has the alias in front
  // of it - so "Criar conta" comes pre-filled instead of asking for it to be retyped.
  initialQuery?: string
}

// Accounts have no name of their own; identify one by its holder, falling back to
// login, then a registered alias, then a short id. Mirrors the grid's labelling so
// the same account reads the same way in the filter and in the spreadsheet.
function accountLabel(account: Account, people: Person[]): string {
  const holder = people.find((p) => p.id === account.holder_id)
  return (
    holder?.name ??
    account.login ??
    account.nicknames[0]?.nickname ??
    account.id.slice(0, 8)
  )
}

function matches(account: Account, people: Person[], query: string): boolean {
  const needle = normalizeSearch(query)
  if (!needle) return true

  return (
    normalizeSearch(accountLabel(account, people)).includes(needle) ||
    normalizeSearch(account.loyalty_program.name).includes(needle) ||
    normalizeSearch(account.loyalty_program.code).includes(needle) ||
    account.nicknames.some((n) => normalizeSearch(n.nickname).includes(needle))
  )
}

export function AccountCombobox({
  accounts,
  people,
  value,
  onChange,
  placeholder = "Buscar por titular, login ou programa...",
  disabled,
  onCreate,
  initialQuery = "",
}: AccountComboboxProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState(initialQuery)

  const selected = accounts.find((a) => a.id === value)
  const filtered = useMemo(
    () => accounts.filter((a) => matches(a, people, query)),
    [accounts, people, query],
  )

  // Offered only when nothing already answers to that name, compared without accents:
  // an operator who types "conceicao" for an existing "Conceição" must be handed the
  // account they meant, not an invitation to register a second one.
  const typed = query.trim()
  const showCreate =
    onCreate !== undefined &&
    typed !== "" &&
    !accounts.some(
      (account) =>
        normalizeSearch(accountLabel(account, people)) ===
          normalizeSearch(typed) ||
        account.nicknames.some(
          (n) => normalizeSearch(n.nickname) === normalizeSearch(typed),
        ),
    )

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

  // The create entry sits after the matches, so the last navigable index is it - which
  // is what lets Enter register an account whose name matched nobody.
  const { activeIndex, listRef, handleKeyDown } = useComboboxNavigation({
    itemCount: filtered.length + (showCreate ? 1 : 0),
    resetKey: open ? query : null,
    onSelect: (index) => {
      const option = filtered[index]
      if (option) handleSelect(option.id)
      else if (showCreate) handleCreate()
    },
  })

  // Re-seeded on every open rather than once: the popover keeps its state between
  // openings, and an operator who cleared the box expects it back the next time.
  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) setQuery(initialQuery)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className="w-full justify-between bg-slate-200 font-normal hover:bg-slate-200"
        >
          <span
            className={cn(
              "flex min-w-0 items-center gap-2",
              !selected && "text-muted-foreground",
            )}
          >
            {selected && (
              <span
                className="h-2.5 w-2.5 shrink-0 rounded-full border border-slate-300"
                style={{ backgroundColor: selected.loyalty_program.color }}
              />
            )}
            <span className="truncate">
              {selected ? accountLabel(selected, people) : placeholder}
            </span>
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
      <PopoverContent
        className="w-(--radix-popover-trigger-width) p-2"
        align="start"
      >
        <div className="relative mb-2">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2" />
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Titular, login ou programa"
            className="pl-8"
          />
        </div>
        <div ref={listRef} className="max-h-56 overflow-y-auto">
          {filtered.length === 0 && !showCreate && (
            <p className="text-muted-foreground px-2 py-3 text-center text-sm">
              Nenhuma conta encontrada.
            </p>
          )}
          {filtered.map((account, index) => (
            <button
              key={account.id}
              type="button"
              onClick={() => handleSelect(account.id)}
              className={cn(
                "hover:bg-accent flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm",
                index === activeIndex && "bg-slate-200",
              )}
            >
              <span
                className="h-2.5 w-2.5 shrink-0 rounded-full border border-slate-300"
                style={{ backgroundColor: account.loyalty_program.color }}
              />
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate">
                  {accountLabel(account, people)}
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {account.loyalty_program.name}
                  {/* Only the merge picker ever asks for archived accounts, and there
                      they are the common case - saying so stops one reading as live. */}
                  {account.deleted_at && " · arquivada"}
                </span>
              </span>
              {account.id === value && <Check className="h-4 w-4 shrink-0" />}
            </button>
          ))}

          {showCreate && (
            <button
              type="button"
              onClick={handleCreate}
              className={cn(
                "hover:bg-accent flex w-full items-start gap-1.5 rounded-sm px-2 py-2 text-left",
                filtered.length > 0 && "mt-1 border-t",
                activeIndex === filtered.length && "bg-slate-200",
              )}
            >
              <Plus className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span className="min-w-0">
                <span className="block truncate text-sm">
                  Criar conta &quot;{typed}&quot;
                </span>
                <span className="text-muted-foreground block text-xs">
                  cadastra a pessoa, a conta e o apelido
                </span>
              </span>
            </button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
