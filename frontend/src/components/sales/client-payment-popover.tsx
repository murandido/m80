"use client"

import { useState } from "react"
import { Wallet } from "lucide-react"
import { toast } from "sonner"
import { createClientPaymentAction } from "@/actions/sale"
import { Button } from "@/components/ui/button"
import { DateInput } from "@/components/ui/date-input"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Spinner } from "@/components/ui/spinner"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"

export interface ClientPaymentRecorded {
  client_payment_id: string
  applied_cents: number
  settled_sales: number
  credit_cents: number
}

interface ClientPaymentPopoverProps {
  clientId: string
  clientName: string | undefined
  // The table owns the confirmation: it is also what offers the undo, on the toast and
  // on Ctrl+Z, alongside every other change made on the screen.
  onRecorded: (result: ClientPaymentRecorded) => void
}

// Today in the operator's own calendar, the day they mean when they change nothing. en-CA
// renders as YYYY-MM-DD, the format the API takes.
function today(): string {
  return new Date().toLocaleDateString("en-CA", { timeZone: "America/Sao_Paulo" })
}

function plural(count: number, one: string, many: string): string {
  return count === 1 ? one : many
}

// What the lump sum did, in one line: the operator typed one number and several rows
// changed, so the confirmation has to say where the money went.
export function summarizeClientPayment(result: ClientPaymentRecorded): string {
  const parts: string[] = []
  if (result.applied_cents > 0) {
    parts.push(`${formatCurrencyBRL(result.applied_cents)} abatidos nas emissões em aberto`)
  }
  if (result.settled_sales > 0) {
    parts.push(
      `${result.settled_sales} ${plural(result.settled_sales, "emissão quitada", "emissões quitadas")}`,
    )
  }
  if (result.credit_cents > 0) {
    parts.push(`crédito do cliente: ${formatCurrencyBRL(result.credit_cents)}`)
  }
  return parts.join(" · ") || "Pagamento registrado."
}

// The "Pagamento parcial" button of the table's footer: how much came in from the client,
// and on which day. The backend spreads it over the client's open sales, oldest first, and
// keeps what is left as credit for the next ones.
export function ClientPaymentPopover({ clientId, clientName, onRecorded }: ClientPaymentPopoverProps) {
  const [open, setOpen] = useState(false)
  const [amount, setAmount] = useState("")
  const [paidOn, setPaidOn] = useState(today())
  const [busy, setBusy] = useState(false)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setAmount("")
      setPaidOn(today())
    }
  }

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    const amountCents = parseCurrencyBRLInput(amount)
    if (amountCents === null || amountCents <= 0) {
      toast.error("Informe quanto o cliente pagou.")
      return
    }
    if (!paidOn) {
      toast.error("Informe o dia em que o dinheiro entrou.")
      return
    }

    setBusy(true)
    const response = await createClientPaymentAction({
      client_id: clientId,
      amount_cents: amountCents,
      paid_on: paidOn,
    })
    setBusy(false)

    if ("error" in response) {
      toast.error(response.error)
      return
    }

    setOpen(false)
    onRecorded(response.result)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={!clientId}
          title={clientId ? undefined : "Escolha o cliente primeiro"}
          className="w-full bg-white font-normal text-slate-600"
        >
          <Wallet className="h-4 w-4" />
          Pagamento parcial
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-3" align="center">
        <form onSubmit={(event) => void handleSubmit(event)} className="flex flex-col gap-3">
          <p className="text-sm">
            Quanto <span className="font-medium">{clientName ?? "o cliente"}</span> pagou?
          </p>
          <div className="flex flex-col gap-1">
            <Label htmlFor="client-payment-amount" className="text-xs text-slate-500">
              Valor recebido (R$)
            </Label>
            <Input
              id="client-payment-amount"
              autoFocus
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              placeholder="Ex: 1500,00"
              inputMode="decimal"
              className="bg-slate-100"
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="client-payment-day" className="text-xs text-slate-500">
              Recebido em
            </Label>
            <DateInput
              id="client-payment-day"
              value={paidOn}
              onChange={setPaidOn}
              className="[&_input]:bg-slate-100"
            />
          </div>
          <p className="text-muted-foreground text-xs">
            Abate nas emissões em aberto do cliente, da mais antiga para a mais nova. O que
            sobrar fica de crédito para as próximas. Dá para desfazer pelo Histórico.
          </p>
          <Button type="submit" disabled={busy} className="bg-slate-800 hover:bg-slate-900">
            {busy && <Spinner className="h-4 w-4" />}
            Registrar pagamento
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}
