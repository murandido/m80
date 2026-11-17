"use client"

import { useMemo, useState } from "react"
import { Check, ChevronsUpDown, Search } from "lucide-react"
import { Button } from "@/components/ui/button"
import { FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useComboboxNavigation } from "@/hooks/use-combobox-navigation"
import { normalizeSearch } from "@/lib/search"
import { cn } from "@/lib/utils"

// One Azul/Smiles account a statement client can be granted, already resolved to words on
// the server (holder name, program, login): the users page is the only place this is picked,
// so the three picker feeds are joined there once instead of shipped to the browser.
export interface StatementAccountChoice {
  id: string
  label: string
  detail: string
}

// The accounts whose statement a statement client may read. Shared by the create form and the
// edit panel, and only rendered while the role being set is statement_client.
export function UserAccountsField({
  accounts,
  value,
  onChange,
  disabled,
}: {
  accounts: StatementAccountChoice[]
  value: string[]
  onChange: (ids: string[]) => void
  disabled?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")

  const filtered = useMemo(() => {
    const needle = normalizeSearch(query)
    if (!needle) return accounts
    return accounts.filter(
      (account) =>
        normalizeSearch(account.label).includes(needle) ||
        normalizeSearch(account.detail).includes(needle),
    )
  }, [accounts, query])

  function toggle(id: string) {
    // Stays open: granting a client both of their accounts is the usual case.
    onChange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id])
  }

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
      ? "Nenhuma conta"
      : value.length === 1
        ? (accounts.find((a) => a.id === value[0])?.label ?? "1 conta")
        : `${value.length} contas`

  return (
    <>
      <FieldLabel>Contas</FieldLabel>
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
              placeholder="Titular, programa ou login"
              className="pl-8"
            />
          </div>
          <div ref={listRef} className="max-h-56 overflow-y-auto">
            {filtered.length === 0 && (
              <p className="px-2 py-3 text-center text-sm text-muted-foreground">
                Nenhuma conta Azul ou Smiles encontrada.
              </p>
            )}
            {filtered.map((account, index) => {
              const checked = value.includes(account.id)
              return (
                <button
                  key={account.id}
                  type="button"
                  role="option"
                  aria-selected={checked}
                  onClick={() => toggle(account.id)}
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
                    <span className="truncate">{account.label}</span>
                    <span className="truncate text-xs text-muted-foreground">{account.detail}</span>
                  </span>
                </button>
              )
            })}
          </div>
        </PopoverContent>
      </Popover>
      <p className="text-xs text-slate-500">
        O cliente só vê o extrato destas contas, sem emissões nem custos.
      </p>
    </>
  )
}
