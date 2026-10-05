"use client"

import { useState } from "react"
import { History } from "lucide-react"
import { toast } from "sonner"
import { getClientPaymentsAction, undoClientPaymentAction } from "@/actions/sale"
import { NO_CLIENT } from "@/app/dashboard/sales/period-params"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Spinner } from "@/components/ui/spinner"
import { formatCurrencyBRL } from "@/lib/currency"
import { cn } from "@/lib/utils"
import type { ClientPaymentHistory, ClientPaymentHistoryItem } from "@/schemas/sale"

interface ClientPaymentsHistoryPopoverProps {
  // The client whose whole history to show. Empty shows every filtered client's payments
  // received in the period instead - see `across`.
  clientId: string
  clientName: string | undefined
  across: {
    // The client filter's ids; empty is every client of the agency.
    clientIds: string[]
    agencyId: string
    dateFrom: string
    dateTo: string
  }
  // Called after a payment is undone, so the table picks up the sales it moved and drops
  // the undo it was still offering for that payment.
  onUndone: (clientPaymentId: string) => void
}

// YYYY-MM-DD as the operator reads it, without a Date round trip that could shift the day.
function formatPaidOn(day: string): string {
  const [year, month, date] = day.split("-")
  return `${date}/${month}/${year}`
}

// The client's lump sums, oldest first like the table, with where each one's money sits.
// With no single client it is the period's payments across clients, newest first.
// It is the way back from a wrong amount: undoing a payment here takes it off every sale
// and out of the credit, and the row stays, struck through, to say what happened.
export function ClientPaymentsHistoryPopover({
  clientId,
  clientName,
  across,
  onUndone,
}: ClientPaymentsHistoryPopoverProps) {
  const [open, setOpen] = useState(false)
  const [history, setHistory] = useState<ClientPaymentHistory | null>(null)
  const [loading, setLoading] = useState(false)
  // Undoing takes two clicks: the first arms the row, the second undoes. A payment that
  // really came in and is undone by a stray click would have to be typed again.
  const [armedId, setArmedId] = useState<string | null>(null)
  const [undoingId, setUndoingId] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    const response = await getClientPaymentsAction(
      clientId ? { clientId } : across,
    )
    setLoading(false)
    if ("error" in response) {
      toast.error(response.error)
      return
    }
    setHistory(response.history)
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    setArmedId(null)
    if (next) {
      setHistory(null)
      void load()
    }
  }

  async function handleUndo(payment: ClientPaymentHistoryItem) {
    if (armedId !== payment.id) {
      setArmedId(payment.id)
      return
    }
    setArmedId(null)
    setUndoingId(payment.id)
    const response = await undoClientPaymentAction(payment.id)
    setUndoingId(null)
    if ("error" in response) {
      toast.error(response.error)
      return
    }
    toast.success(`Pagamento de ${formatCurrencyBRL(payment.amount_cents)} desfeito.`)
    onUndone(payment.id)
    void load()
  }

  const realClientIds = across.clientIds.filter((id) => id !== NO_CLIENT)
  const title = clientId
    ? (clientName ?? "Cliente")
    : realClientIds.length === 0
      ? "Todos os clientes · no período"
      : `${realClientIds.length} ${realClientIds.length === 1 ? "cliente" : "clientes"} · no período`

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          title={clientId ? undefined : "Pagamentos recebidos no período"}
          className="w-full bg-white font-normal text-slate-600"
        >
          <History className="h-4 w-4" />
          Histórico
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96 p-0" align="end">
        <div className="flex items-baseline justify-between gap-3 border-b px-3 py-2">
          <p className="truncate text-sm font-medium">{title}</p>
          {history && (
            <p className="shrink-0 text-xs text-slate-500">
              Crédito:{" "}
              <span className="font-medium text-slate-900 tabular-nums">
                {formatCurrencyBRL(history.credit_cents)}
              </span>
            </p>
          )}
        </div>

        {loading && !history && (
          <div className="flex justify-center py-6">
            <Spinner className="h-5 w-5" />
          </div>
        )}

        {history && history.payments.length === 0 && (
          <p className="text-muted-foreground px-3 py-6 text-center text-sm">
            {clientId
              ? "Nenhum pagamento registrado para esse cliente."
              : "Nenhum pagamento recebido no período."}
          </p>
        )}

        {history && history.payments.length > 0 && (
          <ul className="max-h-80 divide-y overflow-auto">
            {history.payments.map((payment) => {
              const undone = payment.deleted_at !== null
              const spread = payment.allocations.reduce((sum, a) => sum + a.amount_cents, 0)
              const leftover = payment.amount_cents - spread

              return (
                <li key={payment.id} className="flex flex-col gap-1 px-3 py-2 text-sm">
                  <div className="flex items-center justify-between gap-2">
                    <span className={cn("tabular-nums", undone && "text-slate-400 line-through")}>
                      {formatPaidOn(payment.paid_on)} ·{" "}
                      <span className="font-medium">{formatCurrencyBRL(payment.amount_cents)}</span>
                    </span>
                    {!undone && (
                      <Button
                        type="button"
                        size="sm"
                        variant={armedId === payment.id ? "destructive" : "ghost"}
                        disabled={undoingId !== null}
                        onClick={() => void handleUndo(payment)}
                        onBlur={() => setArmedId((id) => (id === payment.id ? null : id))}
                        className="h-7 px-2 text-xs"
                      >
                        {undoingId === payment.id && <Spinner className="h-3 w-3" />}
                        {armedId === payment.id ? "Confirmar" : "Desfazer"}
                      </Button>
                    )}
                  </div>
                  {!clientId && payment.client_name && (
                    <p className="truncate text-xs font-medium text-slate-700">
                      {payment.client_name}
                    </p>
                  )}
                  <p className="text-xs text-slate-500">
                    {undone
                      ? `Desfeito por ${payment.deleted_by_name ?? "alguém"} · lançado por ${payment.created_by_name}`
                      : `Lançado por ${payment.created_by_name}`}
                  </p>
                  {!undone && (
                    <ul className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-slate-600">
                      {payment.allocations.map((allocation) => (
                        <li key={allocation.sale_id} className="tabular-nums">
                          {allocation.program_locator}: {formatCurrencyBRL(allocation.amount_cents)}
                        </li>
                      ))}
                      {leftover > 0 && (
                        <li className="tabular-nums">Crédito: {formatCurrencyBRL(leftover)}</li>
                      )}
                    </ul>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  )
}
