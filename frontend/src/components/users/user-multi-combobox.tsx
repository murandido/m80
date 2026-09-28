"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Search } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useComboboxNavigation } from "@/hooks/use-combobox-navigation"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"
import { User } from "@/schemas/user"

interface UserMultiComboboxProps {
  users: User[]
  // The selected issuer ids. Empty means "no issuer selected", which shows nothing - not
  // every issuer. Selecting all of them is what means "everyone".
  value: string[]
  onChange: (ids: string[]) => void
  disabled?: boolean
}

function matches(user: User, query: string): boolean {
  const needle = normalizeSearch(query)
  if (!needle) return true

  return (
    normalizeSearch(user.name).includes(needle) ||
    normalizeSearch(user.phone).includes(needle)
  )
}

function sameSelection(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  const set = new Set(a)
  return b.every((id) => set.has(id))
}

// Sibling of UserCombobox rather than a `multiple` prop on it, for the same reason
// AgencyMultiCombobox is a sibling of AgencyCombobox.
//
// Unlike AgencyMultiCombobox, the selection is committed on close rather than per toggle: the
// issuances page keys its grid on the resolved filters, so every committed change remounts this
// bar and would slam the popover shut after each pick. A local draft keeps the popover open
// while the operator ticks several issuers and fires a single change when they close it.
export function UserMultiCombobox({
  users,
  value,
  onChange,
  disabled,
}: UserMultiComboboxProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  // The in-progress selection while the popover is open; committed on close.
  const [draft, setDraft] = useState<string[]>(value)

  const filtered = useMemo(
    () => users.filter((u) => matches(u, query)),
    [users, query],
  )

  function handleOpenChange(next: boolean) {
    if (next) {
      setDraft(value)
      setQuery("")
    } else if (!sameSelection(draft, value)) {
      onChange(draft)
    }
    setOpen(next)
  }

  function toggle(id: string) {
    setDraft((current) =>
      current.includes(id) ? current.filter((v) => v !== id) : [...current, id],
    )
  }

  const { activeIndex, listRef, handleKeyDown } = useComboboxNavigation({
    itemCount: filtered.length,
    resetKey: open ? query : null,
    onSelect: (index) => {
      const option = filtered[index]
      if (option) toggle(option.id)
    },
  })

  // The label describes the committed value, not the draft: the trigger only updates once the
  // change lands, which is on close. None reads as "Nenhum" and a full set as "Todos", the two
  // ends the operator can reach with the buttons below.
  const label =
    value.length === 0
      ? "Nenhum emissor"
      : users.length > 0 && value.length >= users.length
        ? "Todos os emissores"
        : value.length === 1
          ? (users.find((u) => u.id === value[0])?.name ?? "1 emissor")
          : `${value.length} emissores`

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
            placeholder="Nome"
            className="pl-8"
          />
        </div>
        <div ref={listRef} className="max-h-56 overflow-y-auto">
          {filtered.length === 0 && (
            <p className="px-2 py-3 text-center text-sm text-muted-foreground">
              Nenhum usuário encontrado.
            </p>
          )}
          {filtered.map((user, index) => {
            const checked = draft.includes(user.id)
            return (
              <button
                key={user.id}
                type="button"
                role="option"
                aria-selected={checked}
                onClick={() => toggle(user.id)}
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
                <span className="min-w-0 flex-1 truncate">{user.name}</span>
              </button>
            )
          })}
        </div>
        {/* The two ends of the range, applied on close like every other pick. */}
        <div className="mt-2 flex items-center justify-between gap-2 border-t border-slate-200 pt-2">
          <button
            type="button"
            disabled={draft.length >= users.length}
            onClick={() => setDraft(users.map((u) => u.id))}
            className="rounded-sm px-2 py-1 text-sm text-slate-600 hover:bg-accent disabled:cursor-default disabled:text-slate-300 disabled:hover:bg-transparent"
          >
            Selecionar todos
          </button>
          <button
            type="button"
            disabled={draft.length === 0}
            onClick={() => setDraft([])}
            className="rounded-sm px-2 py-1 text-sm text-slate-600 hover:bg-accent disabled:cursor-default disabled:text-slate-300 disabled:hover:bg-transparent"
          >
            Limpar
          </button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
