"use client"

import { Wallet } from "lucide-react"
import { formatCurrencyBRL } from "@/lib/currency"
import { cn } from "@/lib/utils"
import type { Sale } from "@/schemas/sale"

export type SaleStatus =
  | "paid"
  | "partial"
  | "unpaid"
  | "refund"
  | "cancelled"
  | "no-value"

// What the row is, in the words the button uses. The balance is signed and already
// accounts for a cancellation (a cancelled ticket is no debt), so a cancelled sale only
// ever has money to hand back, or nothing at all.
export function saleStatus(sale: Sale): SaleStatus {
  if (sale.cancelled_at !== null) {
    return sale.balance_cents < 0 ? "refund" : "cancelled"
  }
  // An issuance still missing its points or its price is worth nothing by arithmetic, not
  // because it is settled - calling it paid would hide the row that most needs filling in.
  if (sale.amount_cents <= 0) return "no-value"
  if (sale.balance_cents <= 0) return "paid"
  return sale.paid_cents > 0 ? "partial" : "unpaid"
}

// The colour of the row's left rule, one per status, matching the button.
export const STATUS_RULE: Record<SaleStatus, string> = {
  paid: "shadow-emerald-600",
  partial: "shadow-amber-400",
  unpaid: "shadow-red-600",
  refund: "shadow-slate-400",
  cancelled: "shadow-slate-300",
  "no-value": "shadow-slate-300",
}

interface SaleStatusButtonProps {
  sale: Sale
  // true settles whatever is missing (or hands back what a cancelled sale holds); false
  // clears the sale's statement.
  onToggle: (paid: boolean) => void
  disabled?: boolean
}

// One click flips a sale between Pago and A Pagar. Partial amounts are not typed here: they
// come in through the client payment bar, which spreads a lump sum over the client's sales
// and leaves at most one of them partly paid - the yellow one.
// YYYY-MM-DD to dd/mm, without a Date round trip that could shift the day.
function formatShortDay(day: string): string {
  const [, month, date] = day.split("-")
  return `${date}/${month}`
}

export function SaleStatusButton({ sale, onToggle, disabled }: SaleStatusButtonProps) {
  const status = saleStatus(sale)
  // Paid entirely by the client's lump sums: the switch has nothing of its own to take
  // back, and the backend refuses it. The way back is undoing the payment in the history.
  const coveredByClientPayment =
    status === "paid" &&
    sale.client_payment_cents > 0 &&
    sale.paid_cents - sale.client_payment_cents <= 0
  const inert = status === "cancelled" || status === "no-value"
  // A partly paid sale is where a client's lump sum ran out: only another client payment
  // moves it, and the backend refuses the switch on it too. Not when what is missing came
  // from an alteration made after the booking was paid: that is a new charge, settled by
  // hand like any other (salePartlyPaid on the backend draws the same line).
  const bookingCovered = sale.paid_cents >= sale.amount_cents - sale.adjustment_cents
  const locked = coveredByClientPayment || (status === "partial" && !bookingCovered)

  const paidShare =
    sale.amount_cents > 0
      ? Math.min(100, Math.max(0, (sale.paid_cents / sale.amount_cents) * 100))
      : 0

  const label = {
    paid: "Pago",
    partial: `Ainda devendo ${formatCurrencyBRL(sale.balance_cents)}`,
    unpaid: "A Pagar",
    refund: `Devolver ${formatCurrencyBRL(-sale.balance_cents)}`,
    cancelled: "Cancelada",
    "no-value": "Sem valor",
  }[status]

  const hint = {
    paid: coveredByClientPayment
      ? `Paga pelo pagamento do cliente de ${formatCurrencyBRL(sale.client_payment_cents)}` +
        (sale.client_payment_paid_on ? ` em ${formatShortDay(sale.client_payment_paid_on)}` : "") +
        " — desfaça no Histórico"
      : "Clique para voltar a A Pagar",
    partial: `${Math.round(paidShare)}% pago — só um novo pagamento parcial altera`,
    unpaid: "Clique para marcar como pago",
    refund: "Clique para registrar a devolução ao cliente",
    cancelled: "Emissão cancelada, nada a receber ou devolver",
    "no-value": "A emissão ainda não tem pontos ou valor do milheiro",
  }[status]

  return (
    <button
      type="button"
      onClick={() => onToggle(status !== "paid")}
      disabled={disabled || inert || locked}
      title={hint}
      aria-label={`${label}. ${hint}`}
      className={cn(
        "relative flex h-9 w-full items-center justify-center gap-2 overflow-hidden rounded-md px-3 text-sm font-medium text-white transition-colors",
        "focus-visible:ring-ring/50 outline-none focus-visible:ring-[3px] disabled:cursor-default",
        status === "paid" && "bg-emerald-600 hover:bg-emerald-700",
        status === "unpaid" && "bg-red-600 hover:bg-red-700",
        // The dark half is what is still owed; the yellow fill grows with what came in.
        status === "partial" && "bg-[#7a5a00]",
        status === "refund" && "bg-slate-500 hover:bg-slate-600",
        inert && "bg-slate-200 text-slate-500",
        disabled && !inert && "opacity-60",
      )}
    >
      {status === "partial" && (
        <span
          aria-hidden
          className="absolute inset-y-0 left-0 bg-amber-400"
          style={{ width: `${paidShare}%` }}
        />
      )}
      <Wallet className="relative h-4 w-4 shrink-0" />
      <span className="relative truncate">{label}</span>
    </button>
  )
}
