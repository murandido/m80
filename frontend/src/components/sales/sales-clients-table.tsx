"use client"

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useTransition } from "react"
import { defaultRangeExtractor, useVirtualizer, type Range } from "@tanstack/react-virtual"
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
import { SALES_PAGE_SIZE, useSalesWindow } from "@/components/sales/use-sales-window"
import { createClientAction } from "@/actions/client"
import {
  parseClientIds,
  serializeClientIds,
  singleClientId,
} from "@/app/dashboard/sales/period-params"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { cn } from "@/lib/utils"
import type { Client } from "@/schemas/client"
import type { Sale } from "@/schemas/sale"

interface SalesClientsTableProps {
  // The listing's filter query, as the page resolved it. The table asks the server for the
  // window of it on screen - see useSalesWindow.
  query: string
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

// What a row is estimated to take before it is measured. Rows grow with wrapping badges and
// become cards below the breakpoint, so every row is measured once it renders.
const ESTIMATED_ROW_PX = 53
// How close to either end of the loaded stretch, in rows, the next page is asked for.
const EDGE_ROWS = 30

function formatDay(iso: string): string {
  return new Date(iso).toLocaleDateString("pt-BR")
}

export function SalesClientsTable({
  query,
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

  // The list is a window over the server's listing: only the pages near what is on screen
  // are in memory, and only the rows on screen are in the DOM.
  const list = useSalesWindow(query)
  const total = list.total ?? 0
  const loaded = list.sales
  const scrollRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  // The sale whose amount is being typed. Its draft lives in the row, so the row has to stay
  // mounted - and its page held - while it is edited, wherever the scroll goes.
  const [draftingSaleId, setDraftingSaleId] = useState<string | null>(null)

  const virtualizer = useVirtualizer({
    // Only the stretch in memory: the scroll is that stretch, like a chat history.
    count: loaded.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_ROW_PX,
    // Keyed by sale, so a row keeps its measured height when pages come in above it.
    getItemKey: (index) => loaded[index]?.id ?? index,
    // Rows mounted above and below the screen, so a quick flick finds them already there.
    overscan: 20,
    // Rows start below the sticky header and the loading rows on top, in the same scroll
    // container.
    scrollMargin: listRef.current?.offsetTop ?? 0,
    rangeExtractor: useCallback(
      (range: Range) => {
        const indexes = defaultRangeExtractor(range)
        for (const id of [draftingSaleId, editingSaleId]) {
          const keep = id ? loaded.findIndex((sale) => sale.id === id) : -1
          if (keep !== -1 && !indexes.includes(keep)) indexes.push(keep)
        }
        return indexes.sort((a, b) => a - b)
      },
      [draftingSaleId, editingSaleId, loaded],
    ),
  })
  const virtualRows = virtualizer.getVirtualItems()

  const { protect, loadOlder, loadNewer } = list
  useEffect(() => {
    protect([draftingSaleId, editingSaleId].filter((id): id is string => Boolean(id)))
  }, [draftingSaleId, editingSaleId, protect])

  // Like a chat: reaching the top of the stretch brings the page before it in; reaching
  // the bottom, the page after. A little ahead of the edge, so the rows are usually there
  // before the scroll gets to them.
  const firstVisible = virtualRows[0]?.index
  const lastVisible = virtualRows[virtualRows.length - 1]?.index
  // Off until the list has settled at its newest sale: before that jump the top rows are
  // the ones on screen, and they would pull older pages in for nothing.
  const [settledFor, setSettledFor] = useState<string | null>(null)
  const settled = settledFor === query
  useEffect(() => {
    if (!settled || firstVisible === undefined || lastVisible === undefined) return
    if (list.hasOlder && firstVisible < EDGE_ROWS) loadOlder()
    if (list.hasNewer && lastVisible > loaded.length - EDGE_ROWS) loadNewer()
  }, [settled, firstVisible, lastVisible, loaded.length, list.hasOlder, list.hasNewer, loadOlder, loadNewer])

  // Pages coming in above (or leaving from the top) move every row down (or up). The row
  // the operator is looking at is put back where it was, so the reading never jumps - the
  // way a chat keeps its place while older messages load. The anchor is taken on every
  // scroll, before the change, and applied right after it renders.
  const anchor = useRef<{ id: string; offset: number } | null>(null)
  useEffect(() => {
    const element = scrollRef.current
    if (!element) return
    const take = () => {
      const top = element.scrollTop
      // The first row still on screen. Item positions already count the header above them.
      const chosen = virtualizer.getVirtualItems().find((row) => row.end > top)
      const sale = chosen ? loaded[chosen.index] : undefined
      anchor.current = chosen && sale ? { id: sale.id, offset: chosen.start - top } : null
    }
    take()
    element.addEventListener("scroll", take, { passive: true })
    return () => element.removeEventListener("scroll", take)
  }, [virtualizer, loaded])

  const shiftedFrom = useRef(list.firstIndex)
  useLayoutEffect(() => {
    if (shiftedFrom.current === list.firstIndex) return
    shiftedFrom.current = list.firstIndex
    const element = scrollRef.current
    const kept = anchor.current
    if (!element || !kept) return
    const index = loaded.findIndex((sale) => sale.id === kept.id)
    const measured = virtualizer.measurementsCache[index]
    if (measured) element.scrollTop = measured.start - kept.offset
  }, [list.firstIndex, loaded, virtualizer])

  // The list opens at its newest sale, once per filter, when the first stretch arrives.
  const anchoredFor = useRef<string | null>(null)
  useEffect(() => {
    if (loaded.length === 0 || anchoredFor.current === query) return
    anchoredFor.current = query
    // The first jump lands on estimated heights; as the rows there are measured, the end
    // moves. Jumping again over a few frames settles on the real bottom.
    const last = loaded.length - 1
    let frames = 0
    let handle = 0
    const settle = () => {
      virtualizer.scrollToIndex(last, { align: "end" })
      if (++frames < 6) handle = requestAnimationFrame(settle)
      else setSettledFor(query)
    }
    settle()
    return () => cancelAnimationFrame(handle)
  }, [loaded.length, query, virtualizer])

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
    // Written into the window before the round trip, so the row changes on the click. A
    // rejected update puts the fields it touched back the way they were.
    const before = Object.fromEntries(
      Object.keys(patch).map((key) => [key, sale[key as keyof Sale]]),
    ) as Partial<Sale>
    list.patchSale(sale.id, patch)

    startTransition(async () => {
      const result = await updateSaleAction(sale.id, payload)
      markSaving(sale.id, false)

      if ("error" in result && result.error) {
        list.patchSale(sale.id, before)
        toast.error(result.error || errorMessage)
        return
      }

      if ("sale" in result && result.sale) list.patchSale(sale.id, result.sale)
      onSaved?.()
      // A payment can move money between a client's sales, so the window is refetched in
      // place; the route refresh brings the client's credit.
      list.refresh()
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

  // The amount to receive, typed as a total. Undo types the previous total back.
  function handleAmount(sale: Sale, amountCents: number) {
    if (amountCents === sale.amount_cents) return
    const previous = sale.amount_cents
    const delta = amountCents - sale.amount_cents
    const patch: Partial<Sale> = {
      amount_cents: amountCents,
      balance_cents: sale.balance_cents + delta,
      price_adjustment_cents: sale.price_adjustment_cents + delta,
    }
    persist(sale, patch, { amount_cents: amountCents }, "Erro ao salvar o valor.", () =>
      pushUndo(() =>
        persist(
          { ...sale, ...patch },
          {
            amount_cents: previous,
            balance_cents: sale.balance_cents,
            price_adjustment_cents: sale.price_adjustment_cents,
          },
          { amount_cents: previous },
          "Erro ao desfazer.",
        ),
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
      list.refresh()
      router.refresh()
    })
  }

  // The footer starts over after a payment: the next one is usually another client's, and
  // a client left selected is how it would land on the wrong one.
  function handleClientPaymentRecorded(result: ClientPaymentRecorded) {
    setPayingClientId("")
    list.refresh()
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
    list.refresh()
    router.refresh()
  }

  const payingClient = clientOptions.find((c) => c.id === payingClientId)

  // Only with one client filtered: summed over several, "what is owed" would be nobody's
  // debt. Signed, like the balance itself - a refund still to go back to the client lowers
  // what they owe, which is how the conversation with them goes.
  const filteredClientId = singleClientId(clientFilter)
  const filteredClient = clientOptions.find((c) => c.id === filteredClientId)
  // Summed by the server over the client's whole list, not the window on screen.
  const owedCents = list.owedCents

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
        ref={scrollRef}
        // Always its own scroll container, in both layouts: the window is measured against it.
        className="max-h-[calc(100vh-17rem)] min-h-64 overflow-auto rounded-lg border bg-white"
      >
        <div
          role="row"
          className={cn(
            ROW_GRID,
            "sticky top-0 z-10 hidden border-b bg-slate-100 text-sm font-medium text-slate-900 @4xl:grid",
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

        {list.error && (
          <p className="py-10 text-center text-sm text-red-700">
            Não foi possível carregar as vendas. Recarregue a página antes de marcar pagamentos.
          </p>
        )}

        {list.total === 0 && (
          <p className="text-muted-foreground py-10 text-center text-sm">
            Nenhuma venda no período selecionado.
          </p>
        )}

        {/* Older sales still to come: rows in the shape of the real ones, where they will
            appear, the way a chat shows its history loading above. */}
        {list.hasOlder && <LoadingRows />}

        <div
          ref={listRef}
          className="relative w-full"
          style={{ height: virtualizer.getTotalSize() }}
        >
        {virtualRows.map((virtualRow) => {
          const sale = loaded[virtualRow.index]
          if (!sale) return null
          const position = {
            position: "absolute" as const,
            top: 0,
            left: 0,
            width: "100%",
            transform: `translateY(${virtualRow.start - virtualizer.options.scrollMargin}px)`,
          }
          // The page that just came in above eases in instead of snapping into place.
          const justArrived =
            virtualRow.index < SALES_PAGE_SIZE && Date.now() - list.arrivedAt < 400

          const isSaving = savingIds.has(sale.id)
          const status = saleStatus(sale)
          const isCancelled = sale.cancelled_at !== null

          return (
            <div
              key={sale.id}
              data-index={virtualRow.index}
              ref={virtualizer.measureElement}
              style={position}
              role="row"
              className={cn(
                "border-b",
                // A row that just replaced its placeholder eases in rather than snapping.
                justArrived && "animate-in fade-in-0 duration-300",
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
                className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 font-medium text-slate-900"
              >
                {/* Only the locator is struck through on a cancelled sale: the badges beside it
                    are still true. */}
                <span
                  className={cn(isCancelled && "text-muted-foreground line-through")}
                  title={isCancelled ? "Emissão cancelada" : undefined}
                >
                  {sale.program_locator}
                </span>
                {isCancelled && sale.cancellation_fee_cents > 0 && (
                  <span
                    className="rounded-full bg-slate-200 px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-slate-800 uppercase"
                    title="Multa de cancelamento, paga pelo cliente"
                  >
                    multa
                  </span>
                )}
                {(sale.alteration_day || sale.adjustment_cents !== 0) && (
                  <span
                    className="rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-amber-900 uppercase"
                    title={
                      sale.alteration_day
                        ? `Passagem alterada neste dia (emitida em ${formatDay(sale.issued_at)})`
                        : "Passagem alterada depois da emissão"
                    }
                  >
                    alteração
                  </span>
                )}
              </span>
              {isCancelled ? (
                <span
                  role="cell"
                  className="text-right text-slate-900 tabular-nums @4xl:text-center"
                  title={
                    sale.cancellation_fee_cents > 0
                      ? `Multa de cancelamento (a passagem era ${formatCurrencyBRL(sale.amount_cents)})`
                      : undefined
                  }
                >
                  {/* A cancelled sale with a fee is worth the fee: that is what the client owes
                      for it, and what the button beside it settles or nets against. */}
                  {formatCurrencyBRL(
                    sale.cancellation_fee_cents > 0 ? sale.cancellation_fee_cents : sale.amount_cents,
                  )}
                </span>
              ) : (
                <AmountCell
                  sale={sale}
                  disabled={isSaving}
                  onCommit={(cents) => handleAmount(sale, cents)}
                  onDraftingChange={(drafting) =>
                    setDraftingSaleId(drafting ? sale.id : null)
                  }
                />
              )}
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
        </div>

        {/* Newer sales below the stretch, after scrolling back up a long way. */}
        {list.hasNewer && <LoadingRows />}

        {filteredClientId && total > 0 && (
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
                : "Soma do que falta receber em todas as emissões do filtro, descontadas as devoluções."}
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

// The amount to receive, editable in place. It is the total the client pays: the server keeps
// how far it is from the computed price, and the badge says so. An alteration-day row shows
// the alteration alone, so it is not where the total is typed - its issue-day row is.
function AmountCell({
  sale,
  disabled,
  onCommit,
  onDraftingChange,
}: {
  sale: Sale
  disabled: boolean
  onCommit: (amountCents: number) => void
  // Told when typing starts and ends, so the list keeps this row mounted meanwhile.
  onDraftingChange: (drafting: boolean) => void
}) {
  const [draft, setDraftState] = useState<string | null>(null)
  function setDraft(next: string | null) {
    setDraftState(next)
    onDraftingChange(next !== null)
  }
  const adjusted = sale.price_adjustment_cents !== 0
  const editable = !disabled && !sale.alteration_day
  const computed = sale.computed_amount_cents ?? sale.amount_cents - sale.price_adjustment_cents

  function commit() {
    if (draft === null) return
    const cents = parseCurrencyBRLInput(draft)
    setDraft(null)
    if (cents === null || cents < 0) {
      if (draft.trim() !== "") toast.error("Valor inválido.")
      return
    }
    onCommit(cents)
  }

  if (draft !== null) {
    return (
      <span role="cell" className="text-right @4xl:text-center">
        <input
          autoFocus
          inputMode="decimal"
          aria-label="Valor a receber"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.currentTarget.blur()
            if (event.key === "Escape") setDraft(null)
          }}
          className="w-28 rounded-md border border-slate-300 px-2 py-1 text-right tabular-nums"
        />
      </span>
    )
  }

  return (
    <span
      role="cell"
      className="flex flex-col items-end gap-0.5 text-slate-900 tabular-nums @4xl:items-center"
    >
      <button
        type="button"
        disabled={!editable}
        onClick={() => setDraft((sale.amount_cents / 100).toFixed(2).replace(".", ","))}
        title={
          sale.alteration_day
            ? "O valor total se edita na linha do dia da emissão"
            : "Clique para mudar o valor a receber"
        }
        className={cn(
          "rounded px-1 tabular-nums",
          editable && "cursor-text hover:bg-slate-200",
        )}
      >
        {formatCurrencyBRL(sale.amount_cents)}
      </button>
      {adjusted && (
        <span
          className="rounded-full bg-sky-100 px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-sky-900 uppercase"
          title={`Calculado ${formatCurrencyBRL(computed)} (${sale.price_adjustment_cents > 0 ? "+" : "−"}${formatCurrencyBRL(Math.abs(sale.price_adjustment_cents))})`}
        >
          ajustado
        </span>
      )}
    </span>
  )
}

// A row still on its way from the server, drawn in the shape of a real one - the date, the
// locator, the amount and the three controls - so it reads as loading rather than as an
// empty line. Every cell fits the row's height, which has to match the estimate exactly.
function SkeletonRow() {
  const bar = "rounded bg-slate-200"
  const control = "h-8 w-full rounded-md bg-slate-100"
  return (
    <>
      <span className="flex items-center">
        <span className={cn(bar, "h-3 w-20")} />
      </span>
      <span className="flex items-center">
        <span className={cn(bar, "h-3.5 w-16")} />
      </span>
      <span className="flex items-center justify-end @4xl:justify-center">
        <span className={cn(bar, "h-3.5 w-20")} />
      </span>
      <span className={cn(WIDE_CELL, "hidden @4xl:block")}>
        <span className={cn(control, "block")} />
      </span>
      <span className={cn(WIDE_CELL, "hidden @4xl:block")}>
        <span className={cn(control, "block bg-slate-200")} />
      </span>
      <span className={cn(WIDE_CELL, "hidden @4xl:block")}>
        <span className={cn(control, "block")} />
      </span>
    </>
  )
}

// A few skeleton rows at an edge of the stretch, while the next page is on its way.
function LoadingRows() {
  return (
    <div aria-label="Carregando mais vendas" role="status">
      {[0, 1, 2].map((key) => (
        <div
          key={key}
          style={{ height: ESTIMATED_ROW_PX }}
          className={cn(ROW_GRID, "animate-pulse overflow-hidden border-b")}
        >
          <SkeletonRow />
        </div>
      ))}
    </div>
  )
}
