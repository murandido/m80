"use client"

import { useState } from "react"
import { Calculator, Copy } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { copyToClipboard } from "@/lib/clipboard"
import { formatCurrencyBRL } from "@/lib/currency"
import { formatSignedPaddedPoints } from "@/lib/points"
import {
  buildCalculo,
  issuanceTotalCents,
} from "@/components/issuances/emissao-message"
import { makeDraftRow } from "@/components/issuances/types"
import { parseNumberText } from "@/components/issuances/board/number-parsing"

// A milheiro calculator: points, milheiro and taxes in, the price out - what the operator
// quotes a client before the issuance is closed. Nothing is saved. The total and the
// calculation line come from the same functions the issuances themselves use, so a quote
// and the issuance it turns into never disagree by a rounding.
export function SimulateIssuanceDialog({
  triggerClassName,
}: {
  triggerClassName?: string
}) {
  const [points, setPoints] = useState("")
  const [milheiro, setMilheiro] = useState("")
  const [taxes, setTaxes] = useState("")

  const parsedPoints = parseNumberText(points, "points")
  const parsedMilheiro = parseNumberText(milheiro, "currency")
  const parsedTaxes =
    taxes.trim() === "" ? null : parseNumberText(taxes, "currency")

  // A throwaway row only to reach the shared helpers, which read these three fields.
  const quote = {
    ...makeDraftRow(),
    points: parsedPoints,
    value_of_the_thousand: parsedMilheiro,
    taxes: parsedTaxes,
  }
  const total = issuanceTotalCents(quote)
  const calculo = buildCalculo(quote)

  // Enter walks the three fields the way Tab does: the quote is typed in one go.
  function focusNext(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter") return
    event.preventDefault()
    const inputs = Array.from(
      event.currentTarget.form?.querySelectorAll<HTMLInputElement>("input") ??
        [],
    )
    const next = inputs[inputs.indexOf(event.currentTarget) + 1]
    if (next) next.focus()
    else if ("text" in calculo)
      void copyToClipboard(calculo.text, "Cálculo copiado")
  }

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button type="button" variant="outline" className={triggerClassName}>
          <Calculator className="h-4 w-4" />
          Simular Emissão
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Simular emissão</DialogTitle>
          <DialogDescription>
            O valor de uma emissão para passar ao cliente antes de fechar. Nada
            é salvo.
          </DialogDescription>
        </DialogHeader>
        <form
          className="grid grid-cols-3 gap-3"
          onSubmit={(event) => event.preventDefault()}
        >
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="simulate-points">Pontos</Label>
            <Input
              id="simulate-points"
              autoFocus
              inputMode="decimal"
              placeholder="256"
              title="Em milheiros: 256 = 256.000; aceita conta, como 120+136"
              value={points}
              onChange={(event) => setPoints(event.target.value)}
              onKeyDown={focusNext}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="simulate-milheiro">Milheiro</Label>
            <Input
              id="simulate-milheiro"
              inputMode="decimal"
              placeholder="16,50"
              value={milheiro}
              onChange={(event) => setMilheiro(event.target.value)}
              onKeyDown={focusNext}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="simulate-taxes">Taxas</Label>
            <Input
              id="simulate-taxes"
              inputMode="decimal"
              placeholder="72,64"
              value={taxes}
              onChange={(event) => setTaxes(event.target.value)}
              onKeyDown={focusNext}
            />
          </div>
        </form>

        <div className="rounded-lg bg-slate-100 p-4">
          <p className="text-xs text-slate-500">Total</p>
          <p className="text-2xl font-semibold text-slate-800 tabular-nums">
            {total !== null ? formatCurrencyBRL(total) : "—"}
          </p>
          {parsedPoints !== null && (
            <p className="mt-1 text-xs text-slate-500">
              {formatSignedPaddedPoints(parsedPoints)} pontos
            </p>
          )}
        </div>

        <div className="flex items-center justify-between gap-3">
          <span className="truncate text-sm text-slate-600 tabular-nums">
            {"text" in calculo ? calculo.text : "Preencha pontos e milheiro"}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!("text" in calculo)}
            onClick={() => {
              if ("text" in calculo)
                void copyToClipboard(calculo.text, "Cálculo copiado")
            }}
          >
            <Copy className="h-3.5 w-3.5" />
            Copiar cálculo
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
