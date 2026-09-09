"use client"

import { useState } from "react"
import { Trash2, Wallet } from "lucide-react"
import { toast } from "sonner"
import {
  createSalePaymentAction,
  deleteSalePaymentAction,
  getSalePaymentsAction,
} from "@/actions/sale"
import { Button } from "@/components/ui/button"
import { DateInput } from "@/components/ui/date-input"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Spinner } from "@/components/ui/spinner"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { cn } from "@/lib/utils"
import type { Sale, SalePayment } from "@/schemas/sale"

interface SalePaymentsPopoverProps {
  sale: Sale
  // Called after any write, so the row (and its balance) is refetched from the server
  // rather than recomputed here - the balance rule lives on the backend.
  onChanged: () => void
  disabled?: boolean
}

// Today in the operator's own calendar, which is the day they mean when they type nothing.
// en-CA renders as YYYY-MM-DD, the format the API takes.
function today(): string {
  return new Date().toLocaleDateString("en-CA", {
    timeZone: "America/Sao_Paulo",
  })
}

function formatDay(value: string): string {
  const [year, month, day] = value.slice(0, 10).split("-")
  return `${day}/${month}/${year}`
}

// What the trigger says: the whole state of the sale in three words.
//
// The balance is signed and already accounts for the cancellation - a cancelled ticket is
// not a debt, so what is left on one is only ever money to hand back. That is why a
// settled cancelled sale reads "Sem pendência" and not "Pago": nothing was collected for
// it, and calling that paid would be a different claim.
export function balanceLabel(sale: Sale): string {
  if (sale.balance_cents > 0)
    return `Faltam ${formatCurrencyBRL(sale.balance_cents)}`
  if (sale.balance_cents < 0)
    return `Devolver ${formatCurrencyBRL(-sale.balance_cents)}`
  return sale.cancelled_at === null ? "Pago" : "Sem pendência"
}

// Whether the row needs somebody to act on it. The one case a zero balance does not settle
// is a live sale worth nothing - an issuance still missing its points or its price, whose
// balance is zero by arithmetic rather than because anything was resolved.
export function needsAttention(sale: Sale): boolean {
  if (sale.balance_cents !== 0) return true
  return sale.cancelled_at === null && sale.amount_cents === 0
}

export function SalePaymentsPopover({
  sale,
  onChanged,
  disabled,
}: SalePaymentsPopoverProps) {
  const [open, setOpen] = useState(false)
  const [payments, setPayments] = useState<SalePayment[]>([])
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [amount, setAmount] = useState("")
  const [paidOn, setPaidOn] = useState(today())
  const [note, setNote] = useState("")

  // Fetched when the row is opened rather than with the listing: most rows are never
  // opened, and the totals the table needs already come with it.
  async function loadPayments() {
    setLoading(true)
    const result = await getSalePaymentsAction(sale.id)
    setLoading(false)

    if ("error" in result) {
      toast.error(result.error)
      return
    }
    setPayments(result.payments)
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setAmount("")
      setNote("")
      setPaidOn(today())
      void loadPayments()
    }
  }

  async function record(amountCents: number, noteText?: string) {
    setBusy(true)
    const result = await createSalePaymentAction(sale.id, {
      amount_cents: amountCents,
      paid_on: paidOn,
      note: noteText,
    })
    setBusy(false)

    if ("error" in result) {
      toast.error(result.error)
      return
    }

    setAmount("")
    setNote("")
    await loadPayments()
    onChanged()
  }

  async function handleAdd() {
    const amountCents = parseCurrencyBRLInput(amount)
    if (amountCents === null || amountCents === 0) {
      toast.error("Informe um valor diferente de zero.")
      return
    }
    await record(amountCents, note.trim() || undefined)
  }

  async function handleDelete(paymentId: string) {
    setBusy(true)
    const result = await deleteSalePaymentAction(sale.id, paymentId)
    setBusy(false)

    if ("error" in result) {
      toast.error(result.error)
      return
    }

    await loadPayments()
    onChanged()
  }

  const pending = needsAttention(sale)

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          className={cn(
            "w-full justify-center gap-1.5 font-normal",
            pending ? "text-amber-700" : "text-emerald-700",
          )}
        >
          <Wallet className="h-3.5 w-3.5 shrink-0" />
          {balanceLabel(sale)}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96 p-3">
        <div className="mb-3 flex items-baseline justify-between text-sm">
          <span className="font-medium">Pagamentos</span>
          <span className="text-muted-foreground text-xs tabular-nums">
            {formatCurrencyBRL(sale.paid_cents)} de{" "}
            {formatCurrencyBRL(sale.amount_cents)}
          </span>
        </div>

        {/* The common case is still the client paying it all at once, which used to be a
            single click on a switch. Without this the change would slow the majority down
            for the sake of the minority that pays in parts. */}
        {sale.balance_cents !== 0 && (
          <Button
            type="button"
            size="sm"
            className="mb-3 w-full"
            disabled={busy}
            onClick={() => void record(sale.balance_cents)}
          >
            {sale.balance_cents > 0
              ? `Receber tudo (${formatCurrencyBRL(sale.balance_cents)})`
              : `Devolver tudo (${formatCurrencyBRL(-sale.balance_cents)})`}
          </Button>
        )}

        <div className="max-h-48 space-y-1 overflow-y-auto">
          {loading && (
            <div className="flex justify-center py-3">
              <Spinner className="h-4 w-4" />
            </div>
          )}
          {!loading && payments.length === 0 && (
            <p className="text-muted-foreground py-3 text-center text-xs">
              Nenhum pagamento lançado.
            </p>
          )}
          {payments.map((entry) => (
            <div
              key={entry.id}
              className="hover:bg-accent flex items-start gap-2 rounded-sm px-1 py-1 text-xs"
            >
              <span className="text-muted-foreground w-16 shrink-0 tabular-nums">
                {formatDay(entry.paid_on)}
              </span>
              <span
                className={cn(
                  "w-24 shrink-0 text-right tabular-nums",
                  entry.amount_cents < 0 && "text-amber-700",
                )}
              >
                {formatCurrencyBRL(entry.amount_cents)}
              </span>
              <span className="min-w-0 flex-1">
                {entry.note && (
                  <span className="block truncate">{entry.note}</span>
                )}
                <span className="text-muted-foreground block truncate text-[11px]">
                  {entry.created_by_name}
                </span>
              </span>
              <button
                type="button"
                onClick={() => void handleDelete(entry.id)}
                disabled={busy}
                className="text-muted-foreground shrink-0 rounded-sm p-1 hover:bg-slate-200 hover:text-red-600 disabled:opacity-40"
                aria-label={`Apagar o lançamento de ${formatCurrencyBRL(entry.amount_cents)}`}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>

        <div className="mt-3 space-y-2 border-t pt-3">
          <div className="flex gap-2">
            <Input
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="Valor (ex: 400,00)"
              className="h-8 text-xs"
              // Negative is how a refund is recorded, so the field has to accept a minus.
              inputMode="text"
            />
            <DateInput
              value={paidOn}
              onChange={setPaidOn}
              className="w-36 [&_input]:h-8 [&_input]:text-xs"
            />
          </div>
          <Input
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Observação (opcional)"
            className="h-8 text-xs"
          />
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="w-full"
            disabled={busy}
            onClick={() => void handleAdd()}
          >
            {busy && <Spinner className="h-3.5 w-3.5" />}
            Lançar
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
