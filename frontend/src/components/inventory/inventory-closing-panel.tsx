"use client"

import { useTransition } from "react"
import { toast } from "sonner"
import { AlertTriangle, Lock, LockOpen } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { applyPaddedThousandsMask } from "@/lib/points"
import { formatCurrencyBRL } from "@/lib/currency"
import { closeInventoryPeriodAction, reopenInventoryPeriodAction } from "@/actions/inventory"
import type { InventoryClosing, InventorySettlement } from "@/schemas/inventory"

interface Props {
  closing: InventoryClosing | null
  startsOn: string
  endsOn: string
}

// Point counts read the way they do on the issuances grid: the absolute number, grouped in
// threes. 632.700, not 632,7 - one grammar for points across the panel.
function formatPoints(points: number): string {
  const sign = points < 0 ? "-" : ""
  return sign + applyPaddedThousandsMask(String(Math.abs(points)))
}

// Money is where the colour goes. A figure the eye has to judge - is this earning or
// losing - gets it; a count of points does not, because there is nothing to judge.
function moneyTone(cents: number): string {
  if (cents > 0) return "text-emerald-700 dark:text-emerald-400"
  if (cents < 0) return "text-red-700 dark:text-red-400"
  return "text-slate-500"
}

// A settlement reads as a sentence because that is how it is acted on: somebody transfers
// money to somebody else. A negative amount is the same sentence with the names swapped,
// rather than a minus sign the reader has to interpret.
function settlementSentence(settlement: InventorySettlement): {
  payer: string
  payee: string
  amount: number
} {
  const payer = settlement.agency_name ?? "?"
  const payee = settlement.to_agency_name ?? "?"
  return settlement.amount_cents < 0
    ? { payer: payee, payee: payer, amount: -settlement.amount_cents }
    : { payer, payee, amount: settlement.amount_cents }
}

function Settlement({ settlement }: { settlement: InventorySettlement }) {
  const { payer, payee, amount } = settlementSentence(settlement)
  if (amount === 0) return <span className="text-slate-400">—</span>
  return (
    <span className="whitespace-nowrap">
      <span className="font-medium">{payer}</span>
      <span className="text-slate-500"> paga </span>
      <span className="font-semibold tabular-nums text-amber-800 dark:text-amber-300">
        {formatCurrencyBRL(amount)}
      </span>
      <span className="text-slate-500"> para </span>
      <span className="font-medium">{payee}</span>
    </span>
  )
}

export function InventoryClosingPanel({ closing, startsOn, endsOn }: Props) {
  const [pending, startTransition] = useTransition()

  if (!closing) {
    return (
      <div className="rounded-lg border p-4 text-sm text-slate-500">
        Não foi possível calcular o fechamento.
      </div>
    )
  }

  const showTotals = closing.accounts.length > 1

  function close(accountId: string) {
    startTransition(async () => {
      const result = await closeInventoryPeriodAction(accountId, startsOn, endsOn)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Período fechado. O custeio dele não muda mais.")
    })
  }

  function reopen(periodId: string) {
    startTransition(async () => {
      const result = await reopenInventoryPeriodAction(periodId)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Período reaberto. O custeio volta a seguir os lançamentos.")
    })
  }

  return (
    <div className="flex flex-col gap-4">
      {closing.totals.pending_points > 0 && (
        <Alert className="border-amber-300 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/40">
          <AlertTriangle className="size-4 text-amber-700 dark:text-amber-400" />
          <AlertTitle className="text-amber-900 dark:text-amber-200">
            {formatPoints(closing.totals.pending_points)} pontos sem lote
          </AlertTitle>
          <AlertDescription className="text-amber-800 dark:text-amber-300">
            Foram emitidos pontos que nenhum lote cobre ainda. Lance a compra que faltou e o número
            some sozinho — as emissões não ficam bloqueadas por isso.
          </AlertDescription>
        </Alert>
      )}

      {closing.accounts.map((entry) => (
        <section
          key={entry.account_id}
          className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700"
        >
          {/* The spreadsheet's header blue, and for the same reason: it separates one table
              from the next at a glance on a screen that holds several. */}
          <header className="flex flex-wrap items-center justify-between gap-3 bg-[#2F5496] px-4 py-2.5 text-white">
            <h2 className="flex items-center gap-2 font-semibold">
              {entry.account_label ?? "Fechamento do período"}
              {entry.closed && (
                <Badge className="gap-1 bg-white/20 text-white hover:bg-white/20">
                  <Lock className="size-3" /> Fechado
                </Badge>
              )}
            </h2>
            {entry.closed && entry.closed_period_id ? (
              <Button
                variant="secondary"
                size="sm"
                disabled={pending}
                onClick={() => reopen(entry.closed_period_id!)}
              >
                <LockOpen className="size-4" />
                Reabrir período
              </Button>
            ) : (
              <Button variant="secondary" size="sm" disabled={pending} onClick={() => close(entry.account_id)}>
                <Lock className="size-4" />
                Fechar período
              </Button>
            )}
          </header>

          {entry.unsupported_split && (
            <Alert className="rounded-none border-x-0 border-t-0 border-red-300 bg-red-50 dark:border-red-800 dark:bg-red-950/40">
              <AlertTriangle className="size-4 text-red-700 dark:text-red-400" />
              <AlertTitle className="text-red-900 dark:text-red-200">
                Rateio indefinido: esta conta tem mais de duas agências
              </AlertTitle>
              <AlertDescription className="text-red-800 dark:text-red-300">
                O percentual do lote diz como dividir entre duas partes, não entre três. Os valores de
                acerto abaixo não devem ser usados até o rateio ser declarado por agência.
              </AlertDescription>
            </Alert>
          )}

          {/* Custo and Lucro only: the sale figure was one column of noise between two the
              eye actually stops on. */}
          <div className="grid grid-cols-2 divide-x divide-slate-200 border-b bg-slate-50 dark:divide-slate-700 dark:bg-slate-900/40">
            <div className="px-4 py-3">
              <p className="text-xs font-medium uppercase tracking-wide text-slate-500">Custo</p>
              <p className="text-2xl font-semibold tabular-nums text-amber-800 dark:text-amber-300">
                {formatCurrencyBRL(entry.cost_cents)}
              </p>
            </div>
            <div className="px-4 py-3">
              <p className="text-xs font-medium uppercase tracking-wide text-slate-500">Lucro</p>
              <p className={`text-2xl font-semibold tabular-nums ${moneyTone(entry.profit_cents)}`}>
                {formatCurrencyBRL(entry.profit_cents)}
              </p>
            </div>
          </div>

          {/* The panel's figure against the airline's, which is the only check there is on
              whether the batches and the bookings add up to reality: no statement export
              carries a balance. Points, not money, so they stay in the panel's grey rather
              than in the colours the money columns own. */}
          <div className="flex flex-wrap items-baseline gap-x-6 gap-y-1 border-b bg-slate-50 px-4 py-2 text-sm dark:bg-slate-900/40">
            <span className="text-slate-500">
              Saldo previsto{" "}
              <span className="font-medium tabular-nums text-slate-700 dark:text-slate-200">
                {formatPoints(entry.expected_balance_points)}
              </span>
            </span>
            {entry.real_balance_points === null || entry.real_balance_points === undefined ? (
              <span className="text-slate-400">
                Saldo real não conferido — informe na ficha da conta
              </span>
            ) : (
              <>
                <span className="text-slate-500">
                  Saldo real{" "}
                  <span className="font-medium tabular-nums text-slate-700 dark:text-slate-200">
                    {formatPoints(entry.real_balance_points)}
                  </span>
                  {entry.real_balance_checked_on && (
                    <span className="text-slate-400">
                      {" "}
                      em {entry.real_balance_checked_on.split("-").reverse().join("/")}
                    </span>
                  )}
                </span>
                {entry.expected_balance_points === entry.real_balance_points ? (
                  <span className="text-emerald-700 dark:text-emerald-400">Batem.</span>
                ) : (
                  <span className="text-amber-800 dark:text-amber-300">
                    Diferença de{" "}
                    {formatPoints(Math.abs(entry.expected_balance_points - entry.real_balance_points))}{" "}
                    pontos.
                  </span>
                )}
              </>
            )}
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-slate-100 text-xs uppercase tracking-wide text-slate-600 dark:bg-slate-800 dark:text-slate-300">
                  <th className="px-4 py-2 text-left font-medium">Lote</th>
                  <th className="px-4 py-2 text-right font-medium">Custo</th>
                  <th className="px-4 py-2 text-right font-medium">Lucro</th>
                  <th className="px-4 py-2 text-right font-medium">Saldo</th>
                  <th className="px-4 py-2 text-left font-medium">Acerto</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {entry.batches.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-6 text-center text-slate-500">
                      Nenhum lote foi consumido neste período.
                    </td>
                  </tr>
                ) : (
                  entry.batches.map((batch) => (
                    <tr key={batch.batch_id} className="hover:bg-slate-50 dark:hover:bg-slate-900/40">
                      <td className="px-4 py-2">
                        <span className="font-medium">{batch.origin_name ?? "Saldo de partida"}</span>
                        <span className="text-slate-500">
                          {" "}
                          {batch.acquired_on.split("-").reverse().join("/")}
                        </span>
                      </td>
                      <td className="px-4 py-2 text-right tabular-nums text-amber-800 dark:text-amber-300">
                        {formatCurrencyBRL(batch.charged_cents)}
                      </td>
                      <td
                        className={`px-4 py-2 text-right font-medium tabular-nums ${moneyTone(batch.profit_cents)}`}
                      >
                        {formatCurrencyBRL(batch.profit_cents)}
                      </td>
                      {/* Points, not money: no colour, and the grid's own grammar. */}
                      <td className="px-4 py-2 text-right tabular-nums text-slate-600 dark:text-slate-300">
                        {formatPoints(batch.remaining_points)}
                      </td>
                      <td className="px-4 py-2">
                        {batch.settlements.length === 0 ? (
                          <span className="text-slate-400">—</span>
                        ) : (
                          batch.settlements.map((settlement) => (
                            <div key={settlement.agency_id}>
                              <Settlement settlement={settlement} />
                            </div>
                          ))
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {entry.settlements.length > 0 && (
            <footer className="flex flex-col gap-1 border-t bg-amber-50/60 px-4 py-3 dark:bg-amber-950/20">
              <p className="text-xs font-medium uppercase tracking-wide text-slate-500">
                Acerto do período
              </p>
              {entry.settlements.map((settlement) => (
                <p key={settlement.agency_id + settlement.to_agency_id} className="text-base">
                  <Settlement settlement={settlement} />
                </p>
              ))}
            </footer>
          )}
        </section>
      ))}

      {showTotals && (
        <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
          <header className="bg-[#1F3864] px-4 py-2.5 font-semibold text-white">Consolidado</header>
          <div className="grid grid-cols-2 divide-x divide-slate-200 bg-slate-50 dark:divide-slate-700 dark:bg-slate-900/40">
            <div className="px-4 py-3">
              <p className="text-xs font-medium uppercase tracking-wide text-slate-500">Custo</p>
              <p className="text-2xl font-semibold tabular-nums text-amber-800 dark:text-amber-300">
                {formatCurrencyBRL(closing.totals.cost_cents)}
              </p>
            </div>
            <div className="px-4 py-3">
              <p className="text-xs font-medium uppercase tracking-wide text-slate-500">Lucro</p>
              <p className={`text-2xl font-semibold tabular-nums ${moneyTone(closing.totals.profit_cents)}`}>
                {formatCurrencyBRL(closing.totals.profit_cents)}
              </p>
            </div>
          </div>
          {closing.totals.settlements.length > 0 && (
            <footer className="flex flex-col gap-1 border-t bg-amber-50/60 px-4 py-3 dark:bg-amber-950/20">
              {closing.totals.settlements.map((settlement) => (
                <p key={settlement.agency_id + settlement.to_agency_id} className="text-base">
                  <Settlement settlement={settlement} />
                </p>
              ))}
            </footer>
          )}
        </section>
      )}
    </div>
  )
}
