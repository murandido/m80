"use client"

import { useMemo, useOptimistic, useState, useTransition } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { toast } from "sonner"
import { ClientCombobox } from "@/components/clients/client-combobox"
import { ClientPaymentPopover } from "@/components/sales/client-payment-popover"
import { SaleNoteCell } from "@/components/sales/sale-note-cell"
import {
  SaleStatusButton,
  saleStatus,
  STATUS_RULE,
} from "@/components/sales/sale-status-button"
import {
  ClientColumnFilter,
  StatusColumnFilter,
} from "@/components/sales/sales-column-filters"
import { updateSaleAction } from "@/actions/sale"
import { createClientAction } from "@/actions/client"
import { formatCurrencyBRL } from "@/lib/currency"
import { cn } from "@/lib/utils"
import type { Client } from "@/schemas/client"
import type { Sale } from "@/schemas/sale"

interface SalesClientsTableProps {
  sales: Sale[]
  clients: Client[]
  // The column filters, as the server applied them.
  clientFilter: string
  paidFilter: string
}

// One grid for the header, every row and the footer, so the columns line up without a
// <table>. Below the breakpoint the same cells fall into a two-column card: the table
// needs about 900px, and the sidebar takes a good part of a laptop's width, which is why
// this is a container query on the table's own width and not a viewport breakpoint.
const ROW_GRID = cn(
  "grid grid-cols-2 gap-x-4 gap-y-2 px-3 py-3",
  "@4xl:grid-cols-[minmax(6rem,0.7fr)_minmax(6rem,0.7fr)_minmax(7rem,0.8fr)_minmax(11rem,1.4fr)_minmax(11rem,1.4fr)_minmax(11rem,1.4fr)] @4xl:items-center @4xl:py-2",
)
// Cells that take the card's full width below the breakpoint and one column above it.
const WIDE_CELL = "col-span-2 @4xl:col-span-1"

function formatDay(iso: string): string {
  return new Date(iso).toLocaleDateString("pt-BR")
}

export function SalesClientsTable({
  sales,
  clients,
  clientFilter,
  paidFilter,
}: SalesClientsTableProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const [, startTransition] = useTransition()
  const [savingIds, setSavingIds] = useState<Set<string>>(new Set())
  // Clients created from a row's combobox. The refresh that follows brings them in the
  // `clients` prop, but the combobox has to be able to show the new name immediately.
  const [createdClients, setCreatedClients] = useState<Client[]>([])
  // The footer's client. Starts on the filtered client, since filtering by one client and
  // then registering what they paid is the usual order.
  const [payingClientId, setPayingClientId] = useState(clientFilter)

  const clientOptions = useMemo(() => {
    const known = new Set(clients.map((c) => c.id))
    return [...clients, ...createdClients.filter((c) => !known.has(c.id))]
  }, [clients, createdClients])

  // React drops each patch when the transition that made it ends - which is after the
  // refresh below landed the real rows. Hand-rolled overrides would have to be cleaned up
  // by hand, and on a screen that refreshes itself from the SSE stream a leftover one
  // would hide the change another operator just made to the same sale.
  const [optimisticSales, applyOptimistic] = useOptimistic(
    sales,
    (current: Sale[], update: { saleId: string; patch: Partial<Sale> }) =>
      current.map((sale) =>
        sale.id === update.saleId ? { ...sale, ...update.patch } : sale,
      ),
  )

  // The URL is authoritative here (useStickyFilters puts remembered filters back into it
  // on arrival), so a column filter only has to change its own key.
  function setFilter(key: "client_id" | "paid", value: string) {
    const params = new URLSearchParams(searchParams.toString())
    if (value) params.set(key, value)
    else params.delete(key)
    router.replace(`${pathname}?${params.toString()}`)
  }

  function markSaving(saleId: string, saving: boolean) {
    setSavingIds((prev) => {
      const next = new Set(prev)
      if (saving) next.add(saleId)
      else next.delete(saleId)
      return next
    })
  }

  function persist(
    sale: Sale,
    patch: Partial<Sale>,
    payload: Parameters<typeof updateSaleAction>[1],
    errorMessage: string,
    onSaved?: () => void,
  ) {
    markSaving(sale.id, true)

    startTransition(async () => {
      // Applied before the await so the row updates on the click instead of after the
      // round trip; a rejected update simply never reaches the refresh, and React puts
      // the server's value back when this transition ends.
      applyOptimistic({ saleId: sale.id, patch })

      const result = await updateSaleAction(sale.id, payload)
      markSaving(sale.id, false)

      if ("error" in result && result.error) {
        toast.error(result.error || errorMessage)
        return
      }

      onSaved?.()
      router.refresh()
    })
  }

  function handleClientChange(sale: Sale, clientId: string) {
    const client = clientOptions.find((c) => c.id === clientId)
    persist(
      sale,
      { client_id: clientId || null, client_name: client?.name ?? null },
      { client_id: clientId || null },
      "Erro ao vincular cliente.",
    )
  }

  // Creating and linking are one gesture from the operator's point of view, so a failure
  // to create must not leave the row looking linked - the link is only attempted once the
  // client exists and has an id.
  function handleClientCreate(sale: Sale, name: string) {
    markSaving(sale.id, true)

    startTransition(async () => {
      const result = await createClientAction({ name })
      markSaving(sale.id, false)

      if ("error" in result) {
        toast.error(result.error)
        return
      }

      setCreatedClients((prev) => [...prev, result.client])
      persist(
        sale,
        { client_id: result.client.id, client_name: result.client.name },
        { client_id: result.client.id },
        "Cliente criado, mas não foi possível vinculá-lo à venda.",
      )
    })
  }

  // The optimistic patch only has to look right while the request is in flight; the
  // refresh brings the server's own arithmetic right after.
  function handleToggle(sale: Sale, paid: boolean) {
    const patch: Partial<Sale> = paid
      ? { paid: true, paid_cents: sale.paid_cents + sale.balance_cents, balance_cents: 0 }
      : { paid: false, paid_cents: 0, balance_cents: sale.amount_cents }

    // Going back to A Pagar clears the sale's whole statement, which a stray click should
    // not cost anybody. Undoing records the full amount again in one line - the balance
    // comes back exactly, the individual lines it had do not.
    const onSaved = paid
      ? undefined
      : () =>
          toast(`${sale.program_locator} voltou para A Pagar`, {
            action: {
              label: "Desfazer",
              onClick: () =>
                persist(
                  { ...sale, ...patch },
                  { paid: true, paid_cents: sale.amount_cents, balance_cents: 0 },
                  { paid: true },
                  "Erro ao desfazer.",
                ),
            },
          })

    persist(sale, patch, { paid }, "Erro ao atualizar o pagamento.", onSaved)
  }

  function handleNote(sale: Sale, note: string | null) {
    persist(sale, { note }, { note }, "Erro ao salvar a observação.")
  }

  const payingClient = clientOptions.find((c) => c.id === payingClientId)

  return (
    <div className="@container flex flex-col gap-3">
      {/* The narrow layout has no header row to hang the column filters on. */}
      <div className="flex gap-2 @4xl:hidden">
        <ClientColumnFilter
          label="Cliente"
          clients={clientOptions}
          value={clientFilter}
          onChange={(value) => setFilter("client_id", value)}
        />
        <StatusColumnFilter
          label="Situação"
          value={paidFilter}
          onChange={(value) => setFilter("paid", value)}
        />
      </div>

      <div
        role="table"
        aria-label="Vendas por cliente"
        className="divide-y overflow-hidden rounded-lg border bg-white @4xl:max-h-[calc(100vh-17rem)] @4xl:overflow-auto"
      >
        <div
          role="row"
          className={cn(
            ROW_GRID,
            "sticky top-0 z-10 hidden bg-slate-100 text-sm font-medium text-slate-900 @4xl:grid",
          )}
        >
          <span role="columnheader">Data</span>
          <span role="columnheader">Emissão</span>
          <span role="columnheader" className="text-center">
            Valor
          </span>
          <span role="columnheader" className="flex items-center justify-center gap-1.5">
            Cliente
            <ClientColumnFilter
              clients={clientOptions}
              value={clientFilter}
              onChange={(value) => setFilter("client_id", value)}
            />
          </span>
          <span role="columnheader" className="flex items-center justify-center gap-1.5">
            Situação
            <StatusColumnFilter
              value={paidFilter}
              onChange={(value) => setFilter("paid", value)}
            />
          </span>
          <span role="columnheader" className="text-center">
            Observação
          </span>
        </div>

        {optimisticSales.length === 0 && (
          <p className="text-muted-foreground py-10 text-center text-sm">
            Nenhuma venda no período selecionado.
          </p>
        )}

        {optimisticSales.map((sale) => {
          const isSaving = savingIds.has(sale.id)
          const status = saleStatus(sale)
          const isCancelled = sale.cancelled_at !== null

          return (
            <div
              key={sale.id}
              role="row"
              className={cn(
                ROW_GRID,
                // A 3px rule down the left edge carries the status, so the column can be
                // swept vertically for the rows that still need somebody. Inset shadow
                // rather than a border so it cannot shift the cells by a pixel.
                "shadow-[inset_3px_0_0_0_var(--tw-shadow-color)]",
                STATUS_RULE[status],
                isSaving && "opacity-60",
              )}
            >
              <span
                role="cell"
                className="col-span-2 text-xs text-slate-500 @4xl:col-span-1 @4xl:text-sm"
              >
                {formatDay(sale.issued_at)}
              </span>
              <span
                role="cell"
                className={cn(
                  "font-medium text-slate-900",
                  isCancelled && "text-muted-foreground line-through",
                )}
                title={isCancelled ? "Emissão cancelada" : undefined}
              >
                {sale.program_locator}
              </span>
              <span
                role="cell"
                className="text-right text-slate-900 tabular-nums @4xl:text-center"
              >
                {formatCurrencyBRL(sale.amount_cents)}
              </span>
              <div role="cell" className={WIDE_CELL}>
                <ClientCombobox
                  clients={clientOptions}
                  value={sale.client_id ?? ""}
                  onChange={(clientId) => handleClientChange(sale, clientId)}
                  onCreate={(name) => handleClientCreate(sale, name)}
                  disabled={isSaving}
                  className="border-transparent"
                />
              </div>
              <div role="cell" className={WIDE_CELL}>
                <SaleStatusButton
                  sale={sale}
                  onToggle={(paid) => handleToggle(sale, paid)}
                  disabled={isSaving}
                />
              </div>
              <div role="cell" className={WIDE_CELL}>
                <SaleNoteCell
                  note={sale.note}
                  onSave={(note) => handleNote(sale, note)}
                  disabled={isSaving}
                />
              </div>
            </div>
          )
        })}

        {/* Sticky so the way to register what a client paid stays in reach however long
            the period's list is. */}
        <div role="row" className={cn(ROW_GRID, "sticky bottom-0 bg-slate-100")}>
          <span
            role="cell"
            className="col-span-2 text-sm font-medium text-slate-900 @4xl:col-span-3"
          >
            Registrar pagamento parcial
          </span>
          <div role="cell" className={WIDE_CELL}>
            <ClientCombobox
              clients={clientOptions}
              value={payingClientId}
              onChange={setPayingClientId}
              className="border-transparent"
            />
          </div>
          <div role="cell" className={WIDE_CELL}>
            <ClientPaymentPopover
              clientId={payingClientId}
              clientName={payingClient?.name}
              onRecorded={() => router.refresh()}
            />
          </div>
        </div>
      </div>
    </div>
  )
}
