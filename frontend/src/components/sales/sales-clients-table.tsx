"use client"

import { useCallback, useEffect, useMemo, useOptimistic, useRef, useState, useTransition } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { toast } from "sonner"
import { ClientCombobox } from "@/components/clients/client-combobox"
import {
  ClientPaymentPopover,
  summarizeClientPayment,
  type ClientPaymentRecorded,
} from "@/components/sales/client-payment-popover"
import { ClientPaymentsHistoryPopover } from "@/components/sales/client-payments-history-popover"
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
import { undoClientPaymentAction, updateSaleAction } from "@/actions/sale"
import { createClientAction } from "@/actions/client"
import {
  parseClientIds,
  serializeClientIds,
  singleClientId,
} from "@/app/dashboard/sales/period-params"
import { formatCurrencyBRL } from "@/lib/currency"
import { cn } from "@/lib/utils"
import type { Client } from "@/schemas/client"
import type { Sale } from "@/schemas/sale"

interface SalesClientsTableProps {
  sales: Sale[]
  clients: Client[]
  // The agency the list shows. A client created from a row belongs to it, since a sale
  // and its client share an agency.
  agencyId: string
  // The column filters, as the server applied them. clientFilter is client_ids as the URL
  // carries it: comma-separated, possibly with NO_CLIENT.
  clientFilter: string
  paidFilter: string
  // The filtered client's credit, when exactly one client is filtered and it could be read.
  clientCreditCents: number | null
  // The period on screen, which the history across clients is narrowed to.
  dateFrom: string
  dateTo: string
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

// Long enough to read the toast and reach its button; the sonner default (4s) was not.
const UNDO_TOAST_MS = 5000
// How far back Ctrl+Z reaches. The stack lives in the tab, so this only bounds memory.
const UNDO_LIMIT = 20

interface UndoEntry {
  id: number
  run: () => void
  // Names what the entry undoes when something else can undo it too - a client payment
  // undone from the history must leave the stack, or Ctrl+Z would undo it a second time.
  key?: string
}

// Ctrl+Z belongs to the field being typed in - the note, a combobox search, the amount of
// a payment. Only outside them does it undo the screen's last change.
function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target.tagName === "INPUT" ||
    target.tagName === "TEXTAREA" ||
    target.tagName === "SELECT"
  )
}

function clientPaymentUndoKey(clientPaymentId: string): string {
  return `client-payment:${clientPaymentId}`
}

function formatDay(iso: string): string {
  return new Date(iso).toLocaleDateString("pt-BR")
}

export function SalesClientsTable({
  sales,
  clients,
  agencyId,
  clientFilter,
  paidFilter,
  clientCreditCents,
  dateFrom,
  dateTo,
}: SalesClientsTableProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const [, startTransition] = useTransition()
  const [savingIds, setSavingIds] = useState<Set<string>>(new Set())
  // The row whose client or note popover is open. Those popovers render in a portal, so
  // :focus-within on the row cannot tell that the row is the one being edited.
  const [editingSaleId, setEditingSaleId] = useState<string | null>(null)
  // Clients created from a row's combobox. The refresh that follows brings them in the
  // `clients` prop, but the combobox has to be able to show the new name immediately.
  const [createdClients, setCreatedClients] = useState<Client[]>([])
  // The footer's client. Starts on the filtered client, since filtering by one client and
  // then registering what they paid is the usual order.
  const [payingClientId, setPayingClientId] = useState(singleClientId(clientFilter))
  // The footer's client follows the filter when it changes. Adjusted during render rather
  // than by remounting the table on every filter change: a remount closes the client
  // filter's popover after each tick, and picking several clients is what it is for.
  const [followedFilter, setFollowedFilter] = useState(clientFilter)
  if (followedFilter !== clientFilter) {
    setFollowedFilter(clientFilter)
    setPayingClientId(singleClientId(clientFilter))
  }
  const selectedClientIds = useMemo(() => parseClientIds(clientFilter), [clientFilter])

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

  // Every change made on this screen leaves a way back here: the toast's Desfazer and
  // Ctrl+Z take the same entry, so an undo never runs twice. Undoing does not push an
  // entry of its own - there is no redo.
  const undoStack = useRef<UndoEntry[]>([])
  const undoSequence = useRef(0)

  const undo = useCallback((id?: number): boolean => {
    const stack = undoStack.current
    const entry = id === undefined ? stack[stack.length - 1] : stack.find((e) => e.id === id)
    if (!entry) return false
    undoStack.current = stack.filter((e) => e.id !== entry.id)
    toast.dismiss(`undo-${entry.id}`)
    entry.run()
    return true
  }, [])

  function pushUndo(run: () => void, message?: string, key?: string) {
    const id = ++undoSequence.current
    undoStack.current = [...undoStack.current, { id, run, key }].slice(-UNDO_LIMIT)
    if (message) {
      toast(message, {
        id: `undo-${id}`,
        duration: UNDO_TOAST_MS,
        action: { label: "Desfazer", onClick: () => undo(id) },
      })
    }
  }

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      const isUndo =
        (event.ctrlKey || event.metaKey) && !event.shiftKey && event.key.toLowerCase() === "z"
      if (!isUndo || isEditable(event.target)) return
      if (undo()) event.preventDefault()
    }
    window.addEventListener("keydown", handleKeyDown)
    return () => window.removeEventListener("keydown", handleKeyDown)
  }, [undo])

  // The URL is authoritative here (useStickyFilters puts remembered filters back into it
  // on arrival), so a column filter only has to change its own key.
  function setFilter(key: "client_ids" | "paid", value: string) {
    const params = new URLSearchParams(searchParams.toString())
    if (value) params.set(key, value)
    else params.delete(key)
    // The single client_id an older link carried is folded into client_ids on arrival; left
    // in the URL it would come back the moment the multi-select is emptied.
    params.delete("client_id")
    router.replace(`${pathname}?${params.toString()}`)
  }

  function trackEditing(saleId: string) {
    return (open: boolean) =>
      setEditingSaleId((current) => (open ? saleId : current === saleId ? null : current))
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

  // Linking back to whoever the sale had before. No toast: linking is the most frequent
  // gesture here, and a toast on every one would bury the ones that matter.
  function undoLinkTo(sale: Sale) {
    const previous = { client_id: sale.client_id, client_name: sale.client_name }
    return (linked: Sale) => () =>
      persist(linked, previous, { client_id: previous.client_id }, "Erro ao desfazer.")
  }

  function handleClientChange(sale: Sale, clientId: string) {
    const client = clientOptions.find((c) => c.id === clientId)
    const patch = { client_id: clientId || null, client_name: client?.name ?? null }
    const undoLink = undoLinkTo(sale)
    persist(sale, patch, { client_id: clientId || null }, "Erro ao vincular cliente.", () =>
      pushUndo(undoLink({ ...sale, ...patch })),
    )
  }

  // Creating and linking are one gesture from the operator's point of view, so a failure
  // to create must not leave the row looking linked - the link is only attempted once the
  // client exists and has an id.
  function handleClientCreate(sale: Sale, name: string) {
    markSaving(sale.id, true)

    startTransition(async () => {
      const result = await createClientAction({ name, agency_id: agencyId })
      markSaving(sale.id, false)

      if ("error" in result) {
        toast.error(result.error)
        return
      }

      setCreatedClients((prev) => [...prev, result.client])
      const patch = { client_id: result.client.id, client_name: result.client.name }
      const undoLink = undoLinkTo(sale)
      persist(
        sale,
        patch,
        { client_id: result.client.id },
        "Cliente criado, mas não foi possível vinculá-lo à venda.",
        // Undo unlinks; the client just created stays in the register.
        () => pushUndo(undoLink({ ...sale, ...patch })),
      )
    })
  }

  // The optimistic patch only has to look right while the request is in flight; the
  // refresh brings the server's own arithmetic right after.
  function handleToggle(sale: Sale, paid: boolean) {
    // Going back to A Pagar clears only what the switch can take back - the sale's own
    // lines. Whatever a client payment put there stays, so the sale may land on partly
    // paid rather than open.
    const patch: Partial<Sale> = paid
      ? { paid: true, paid_cents: sale.paid_cents + sale.balance_cents, balance_cents: 0 }
      : {
          paid: false,
          paid_cents: sale.client_payment_cents,
          balance_cents: sale.amount_cents - sale.client_payment_cents,
        }
    const before: Partial<Sale> = {
      paid: sale.paid,
      paid_cents: sale.paid_cents,
      balance_cents: sale.balance_cents,
    }

    // A stray click here costs a payment line, so both directions offer the way back.
    // Undoing A Pagar records what was missing again in one line - the balance comes back
    // exactly, the individual lines it had do not.
    const message = paid
      ? `${sale.program_locator} marcada como paga`
      : `${sale.program_locator} voltou para A Pagar`
    persist(sale, patch, { paid }, "Erro ao atualizar o pagamento.", () =>
      pushUndo(
        () => persist({ ...sale, ...patch }, before, { paid: !paid }, "Erro ao desfazer."),
        message,
      ),
    )
  }

  function handleNote(sale: Sale, note: string | null) {
    const previous = sale.note ?? null
    persist(sale, { note }, { note }, "Erro ao salvar a observação.", () =>
      pushUndo(() =>
        persist({ ...sale, note }, { note: previous }, { note: previous }, "Erro ao desfazer."),
      ),
    )
  }

  function undoClientPayment(clientPaymentId: string) {
    startTransition(async () => {
      const result = await undoClientPaymentAction(clientPaymentId)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Pagamento desfeito.")
      router.refresh()
    })
  }

  // The footer starts over after a payment: the next one is usually another client's, and
  // a client left selected is how it would land on the wrong one.
  function handleClientPaymentRecorded(result: ClientPaymentRecorded) {
    setPayingClientId("")
    router.refresh()
    pushUndo(
      () => undoClientPayment(result.client_payment_id),
      summarizeClientPayment(result),
      clientPaymentUndoKey(result.client_payment_id),
    )
  }

  function handleClientPaymentUndoneFromHistory(clientPaymentId: string) {
    const key = clientPaymentUndoKey(clientPaymentId)
    const entry = undoStack.current.find((e) => e.key === key)
    if (entry) {
      undoStack.current = undoStack.current.filter((e) => e.id !== entry.id)
      toast.dismiss(`undo-${entry.id}`)
    }
    router.refresh()
  }

  const payingClient = clientOptions.find((c) => c.id === payingClientId)

  // Only with one client filtered: summed over several, "what is owed" would be nobody's
  // debt. Signed, like the balance itself - a refund still to go back to the client lowers
  // what they owe, which is how the conversation with them goes.
  const filteredClientId = singleClientId(clientFilter)
  const filteredClient = clientOptions.find((c) => c.id === filteredClientId)
  const owedCents = optimisticSales.reduce((sum, sale) => sum + sale.balance_cents, 0)

  return (
    <div className="@container flex flex-col gap-3">
      {/* The narrow layout has no header row to hang the column filters on. */}
      <div className="flex gap-2 @4xl:hidden">
        <ClientColumnFilter
          label="Cliente"
          clients={clientOptions}
          value={selectedClientIds}
          onChange={(ids) => setFilter("client_ids", serializeClientIds(ids))}
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
              value={selectedClientIds}
              onChange={(ids) => setFilter("client_ids", serializeClientIds(ids))}
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
                // Darker than the white rows around it, so the eye can follow the row the
                // pointer is on, and darker still while one of its popovers is open.
                "transition-colors hover:bg-slate-100",
                editingSaleId === sale.id && "bg-slate-300/60 hover:bg-slate-300/60",
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
                  onOpenChange={trackEditing(sale.id)}
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
                  onOpenChange={trackEditing(sale.id)}
                  disabled={isSaving}
                />
              </div>
            </div>
          )
        })}

        {filteredClientId && optimisticSales.length > 0 && (
          <div
            role="row"
            className={cn(ROW_GRID, "bg-slate-200 text-sm font-medium text-slate-900")}
          >
            <span role="cell" className="col-span-2 @4xl:col-span-2">
              Total devido{filteredClient ? ` por ${filteredClient.name}` : ""}
            </span>
            <span
              role="cell"
              className={cn(
                "text-right font-semibold tabular-nums @4xl:text-center",
                owedCents > 0 ? "text-red-700" : "text-emerald-700",
              )}
            >
              {formatCurrencyBRL(owedCents)}
            </span>
            <span role="cell" className="col-span-2 text-xs font-normal text-slate-600 @4xl:col-span-3">
              {clientCreditCents !== null && clientCreditCents > 0
                ? `Crédito do cliente: ${formatCurrencyBRL(clientCreditCents)} (entra nas próximas emissões)`
                : "Soma do que falta receber nas emissões listadas, descontadas as devoluções."}
            </span>
          </div>
        )}

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
              onRecorded={handleClientPaymentRecorded}
            />
          </div>
          <div role="cell" className={WIDE_CELL}>
            <ClientPaymentsHistoryPopover
              clientId={payingClientId}
              clientName={payingClient?.name}
              across={{ clientIds: selectedClientIds, agencyId, dateFrom, dateTo }}
              onUndone={handleClientPaymentUndoneFromHistory}
            />
          </div>
        </div>
      </div>
    </div>
  )
}
