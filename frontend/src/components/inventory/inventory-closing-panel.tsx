"use client"

import { Fragment, useState, useTransition } from "react"
import { toast } from "sonner"
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Lock,
  LockOpen,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { applyPaddedThousandsMask } from "@/lib/points"
import { formatCurrencyBRL } from "@/lib/currency"
import {
  closeInventoryPeriodAction,
  reopenInventoryPeriodAction,
} from "@/actions/inventory"
import type {
  InventoryBatchResult,
  InventoryClosing,
  InventoryConsumer,
  InventorySettlement,
  InventorySettlementPart,
} from "@/schemas/inventory"

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
      <span className="font-semibold text-amber-800 tabular-nums dark:text-amber-300">
        {formatCurrencyBRL(amount)}
      </span>
      <span className="text-slate-500"> para </span>
      <span className="font-medium">{payee}</span>
    </span>
  )
}

function SettlementList({
  settlements,
}: {
  settlements: InventorySettlement[]
}) {
  if (settlements.length === 0) return <span className="text-slate-400">—</span>
  return (
    <>
      {settlements.map((settlement) => (
        <div key={settlement.agency_id + settlement.to_agency_id}>
          <Settlement settlement={settlement} />
        </div>
      ))}
    </>
  )
}

// How one term of a settlement reads in the calculation. The payer is always named, because
// the sign of the line depends on who owes it.
function partLabel(part: InventorySettlementPart): string {
  const payer = part.payer_agency_name ?? "?"
  const payee = part.payee_agency_name ?? "?"
  const base = formatCurrencyBRL(part.base_cents)
  // A percent the batches disagreed on has no single figure to print.
  const share =
    part.percent === null || part.percent === undefined
      ? "Parte"
      : `${part.percent}%`
  switch (part.kind) {
    case "cost":
      return `Custo dos pontos que a ${payer} usou dos lotes da ${payee}`
    case "profit_share":
      return `${share} do lucro da ${payer} (${base})`
    case "fee_share":
      return `${share} da taxa de resgate economizada pela ${payer} (${base})`
  }
}

const partOrder = { cost: 0, profit_share: 1, fee_share: 2 } as const

// The settlement as a sum anyone can check: each agency's activity, every term with its sign,
// and who passes what to whom at the end. The terms are the ones the settlement was added up
// from, so they reach the total to the centavo.
function SettlementCalculation({
  agencies,
  parts,
  settlements,
}: {
  agencies: InventoryConsumer[]
  parts: InventorySettlementPart[]
  settlements: InventorySettlement[]
}) {
  const showFees = agencies.some(
    (agency) => agency.redemption_fee_saved_cents !== 0,
  )

  return (
    <div className="flex flex-col gap-4">
      {agencies.length > 0 && (
        <table className="w-full max-w-3xl text-sm">
          <thead>
            <tr className="text-left text-xs tracking-wide text-slate-500 uppercase">
              <th className="py-1 pr-4 font-medium">Agência</th>
              <th className="py-1 pr-4 text-right font-medium">
                Pontos emitidos
              </th>
              <th className="py-1 pr-4 text-right font-medium">
                Lucro nos pontos
              </th>
              {showFees && (
                <th className="py-1 text-right font-medium">
                  Taxa de resgate economizada
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {agencies.map((agency) => (
              <tr key={agency.agency_id}>
                <td className="py-1 pr-4 font-medium">
                  {agency.agency_name ?? "?"}
                </td>
                <td className="py-1 pr-4 text-right tabular-nums">
                  {formatPoints(agency.points)}
                </td>
                <td
                  className={`py-1 pr-4 text-right tabular-nums ${moneyTone(agency.profit_cents)}`}
                >
                  {formatCurrencyBRL(agency.profit_cents)}
                </td>
                {showFees && (
                  <td
                    className={`py-1 text-right tabular-nums ${moneyTone(agency.redemption_fee_saved_cents)}`}
                  >
                    {formatCurrencyBRL(agency.redemption_fee_saved_cents)}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {settlements.length === 0 ? (
        <p className="text-sm text-slate-500">Nada a acertar.</p>
      ) : (
        settlements.map((settlement) => {
          const debtor = settlement.agency_id
          const creditor = settlement.to_agency_id
          const terms = parts
            .filter(
              (part) =>
                (part.payer_agency_id === debtor &&
                  part.payee_agency_id === creditor) ||
                (part.payer_agency_id === creditor &&
                  part.payee_agency_id === debtor),
            )
            .sort((a, b) => partOrder[a.kind] - partOrder[b.kind])
          return (
            <div key={debtor + creditor} className="max-w-3xl">
              {terms.length > 0 && (
                <>
                  <p className="mb-1 text-xs font-medium tracking-wide text-slate-500 uppercase">
                    Cálculo
                  </p>
                  <table className="w-full text-sm">
                    <tbody>
                      {terms.map((part, index) => {
                        const owedByDebtor = part.payer_agency_id === debtor
                        return (
                          <tr key={`${part.kind}-${part.payer_agency_id}`}>
                            <td className="w-5 py-0.5 text-slate-500">
                              {index === 0 && owedByDebtor
                                ? ""
                                : owedByDebtor
                                  ? "+"
                                  : "−"}
                            </td>
                            <td className="py-0.5 pr-4 text-slate-700 dark:text-slate-300">
                              {partLabel(part)}
                            </td>
                            <td className="py-0.5 text-right tabular-nums">
                              {formatCurrencyBRL(part.amount_cents)}
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </>
              )}
              <p
                className={`text-base ${terms.length > 0 ? "mt-1 border-t border-slate-300 pt-2 dark:border-slate-600" : ""}`}
              >
                <span className="font-semibold">
                  {settlement.agency_name ?? "?"}
                </span>
                <span className="text-slate-500"> repassa </span>
                <span className="font-semibold text-amber-800 tabular-nums dark:text-amber-300">
                  {formatCurrencyBRL(settlement.amount_cents)}
                </span>
                <span className="text-slate-500"> para </span>
                <span className="font-semibold">
                  {settlement.to_agency_name ?? "?"}
                </span>
              </p>
            </div>
          )
        })
      )}
    </div>
  )
}

// What a batch is made of in the period: every booking and refund that drew on it, with the
// redemption fee each one saved, and the fee subtotal the fee settlement is computed from.
function BatchLines({ batch }: { batch: InventoryBatchResult }) {
  // Counted per booking, net of its refunds: a booking and its reversal are two lines but one
  // emission, and a fully refunded one kept no fee at all.
  const feeByIssuance = new Map<string, number>()
  for (const line of batch.lines) {
    feeByIssuance.set(
      line.issuance_id,
      (feeByIssuance.get(line.issuance_id) ?? 0) +
        line.redemption_fee_saved_cents,
    )
  }
  const withFee = [...feeByIssuance.values()].filter((fee) => fee !== 0).length
  return (
    <div className="flex flex-col gap-3 bg-slate-50/70 px-4 py-3 dark:bg-slate-900/30">
      {batch.lines.length === 0 ? (
        <p className="text-sm text-slate-500">
          Nenhuma emissão deste lote está visível para você.
        </p>
      ) : (
        <table className="w-full text-xs">
          <thead>
            <tr className="text-left tracking-wide text-slate-500 uppercase">
              <th className="py-1 pr-3 font-medium">Data</th>
              <th className="py-1 pr-3 font-medium">Localizador</th>
              <th className="py-1 pr-3 font-medium">Agência</th>
              <th className="py-1 pr-3 text-right font-medium">Pontos</th>
              <th className="py-1 pr-3 text-right font-medium">Custo</th>
              <th className="py-1 pr-3 text-right font-medium">Venda</th>
              <th className="py-1 pr-3 text-right font-medium">Lucro</th>
              <th className="py-1 text-right font-medium">Taxa de resgate</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {batch.lines.map((line, index) => (
              <tr key={`${line.issuance_id}-${index}`}>
                <td className="py-1 pr-3 tabular-nums">
                  {line.occurred_on.split("-").reverse().join("/")}
                </td>
                <td className="py-1 pr-3 font-medium">
                  {line.program_locator}
                  {line.points < 0 && (
                    <span className="ml-1 font-normal text-red-700 dark:text-red-400">
                      reembolso
                    </span>
                  )}
                </td>
                <td className="py-1 pr-3">{line.agency_name ?? "?"}</td>
                <td
                  className={`py-1 pr-3 text-right tabular-nums ${line.points < 0 ? "text-red-700 dark:text-red-400" : ""}`}
                >
                  {formatPoints(line.points)}
                </td>
                <td className="py-1 pr-3 text-right text-amber-800 tabular-nums dark:text-amber-300">
                  {formatCurrencyBRL(line.cost_cents)}
                </td>
                <td className="py-1 pr-3 text-right tabular-nums">
                  {formatCurrencyBRL(line.revenue_cents)}
                </td>
                <td
                  className={`py-1 pr-3 text-right tabular-nums ${moneyTone(line.profit_cents)}`}
                >
                  {formatCurrencyBRL(line.profit_cents)}
                </td>
                <td
                  className={`py-1 text-right tabular-nums ${moneyTone(line.redemption_fee_saved_cents)}`}
                >
                  {line.redemption_fee_saved_cents === 0
                    ? "—"
                    : formatCurrencyBRL(line.redemption_fee_saved_cents)}
                </td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="border-t font-medium">
              <td
                colSpan={7}
                className="py-1.5 pr-3 text-right text-slate-600 dark:text-slate-300"
              >
                Subtotal das taxas de resgate (
                {withFee === 1 ? "1 emissão" : `${withFee} emissões`})
              </td>
              <td
                className={`py-1.5 text-right tabular-nums ${moneyTone(batch.redemption_fee_saved_cents)}`}
              >
                {formatCurrencyBRL(batch.redemption_fee_saved_cents)}
              </td>
            </tr>
          </tfoot>
        </table>
      )}
      <SettlementCalculation
        agencies={batch.consumers}
        parts={batch.parts}
        settlements={batch.total_settlements}
      />
    </div>
  )
}

export function InventoryClosingPanel({ closing, startsOn, endsOn }: Props) {
  const [pending, startTransition] = useTransition()
  // Which batches are open, keyed by account and batch so the same batch id can never
  // collide across two accounts on one screen.
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  function toggle(key: string) {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

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
      const result = await closeInventoryPeriodAction(
        accountId,
        startsOn,
        endsOn,
      )
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
      toast.success(
        "Período reaberto. O custeio volta a seguir os lançamentos.",
      )
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
            Foram emitidos pontos que nenhum lote cobre ainda. Lance a compra
            que faltou e o número some sozinho — as emissões não ficam
            bloqueadas por isso.
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
              <Button
                variant="secondary"
                size="sm"
                disabled={pending}
                onClick={() => close(entry.account_id)}
              >
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
                O percentual do lote diz como dividir entre duas partes, não
                entre três. Os valores de acerto abaixo não devem ser usados até
                o rateio ser declarado por agência.
              </AlertDescription>
            </Alert>
          )}

          {/* Custo and Lucro only: the sale figure was one column of noise between two the
              eye actually stops on. The redemption fee joins them only on an account that
              saved any - a column of zeros on every other account would be the same noise. */}
          <div
            className={`grid divide-x divide-slate-200 border-b bg-slate-50 dark:divide-slate-700 dark:bg-slate-900/40 ${entry.redemption_fee_saved_cents !== 0 ? "grid-cols-3" : "grid-cols-2"}`}
          >
            <div className="px-4 py-3">
              <p className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                Custo
              </p>
              <p className="text-2xl font-semibold text-amber-800 tabular-nums dark:text-amber-300">
                {formatCurrencyBRL(entry.cost_cents)}
              </p>
            </div>
            <div className="px-4 py-3">
              <p className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                Lucro
              </p>
              <p
                className={`text-2xl font-semibold tabular-nums ${moneyTone(entry.profit_cents)}`}
              >
                {formatCurrencyBRL(entry.profit_cents)}
              </p>
            </div>
            {entry.redemption_fee_saved_cents !== 0 && (
              <div className="px-4 py-3">
                <p className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                  Taxa de resgate economizada
                </p>
                <p
                  className={`text-2xl font-semibold tabular-nums ${moneyTone(entry.redemption_fee_saved_cents)}`}
                >
                  {formatCurrencyBRL(entry.redemption_fee_saved_cents)}
                </p>
              </div>
            )}
          </div>

          {/* The panel's figure against the airline's, which is the only check there is on
              whether the batches and the bookings add up to reality: no statement export
              carries a balance. Points, not money, so they stay in the panel's grey rather
              than in the colours the money columns own. */}
          <div className="flex flex-wrap items-baseline gap-x-6 gap-y-1 border-b bg-slate-50 px-4 py-2 text-sm dark:bg-slate-900/40">
            <span className="text-slate-500">
              Saldo previsto{" "}
              <span className="font-medium text-slate-700 tabular-nums dark:text-slate-200">
                {formatPoints(entry.expected_balance_points)}
              </span>
            </span>
            {entry.real_balance_points === null ||
            entry.real_balance_points === undefined ? (
              <span className="text-slate-400">
                Saldo real não conferido — informe na ficha da conta
              </span>
            ) : (
              <>
                <span className="text-slate-500">
                  Saldo real{" "}
                  <span className="font-medium text-slate-700 tabular-nums dark:text-slate-200">
                    {formatPoints(entry.real_balance_points)}
                  </span>
                  {entry.real_balance_checked_on && (
                    <span className="text-slate-400">
                      {" "}
                      em{" "}
                      {entry.real_balance_checked_on
                        .split("-")
                        .reverse()
                        .join("/")}
                    </span>
                  )}
                </span>
                {entry.expected_balance_points === entry.real_balance_points ? (
                  <span className="text-emerald-700 dark:text-emerald-400">
                    Batem.
                  </span>
                ) : (
                  <span className="text-amber-800 dark:text-amber-300">
                    Diferença de{" "}
                    {formatPoints(
                      Math.abs(
                        entry.expected_balance_points -
                          entry.real_balance_points,
                      ),
                    )}{" "}
                    pontos.
                  </span>
                )}
              </>
            )}
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-slate-100 text-xs tracking-wide text-slate-600 uppercase dark:bg-slate-800 dark:text-slate-300">
                  <th className="px-4 py-2 text-left font-medium">Lote</th>
                  <th className="px-4 py-2 text-right font-medium">Custo</th>
                  <th className="px-4 py-2 text-right font-medium">Lucro</th>
                  <th className="px-4 py-2 text-right font-medium">
                    Taxa de resgate
                  </th>
                  <th className="px-4 py-2 text-right font-medium">Saldo</th>
                  <th className="px-4 py-2 text-left font-medium">Acerto</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {entry.batches.length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-4 py-6 text-center text-slate-500"
                    >
                      Nenhum lote foi consumido neste período.
                    </td>
                  </tr>
                ) : (
                  entry.batches.map((batch) => {
                    const key = `${entry.account_id}:${batch.batch_id}`
                    const open = expanded.has(key)
                    return (
                      <Fragment key={batch.batch_id}>
                        <tr
                          className="cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-900/40"
                          onClick={() => toggle(key)}
                          aria-expanded={open}
                          title={
                            open
                              ? "Esconder as emissões deste lote"
                              : "Ver as emissões e reembolsos deste lote"
                          }
                        >
                          <td className="px-4 py-2">
                            {open ? (
                              <ChevronDown className="mr-1 inline size-4 text-slate-500" />
                            ) : (
                              <ChevronRight className="mr-1 inline size-4 text-slate-500" />
                            )}
                            <span className="font-medium">
                              {batch.origin_name ?? "Saldo de partida"}
                            </span>
                            <span className="text-slate-500">
                              {" "}
                              {batch.acquired_on.split("-").reverse().join("/")}
                            </span>
                          </td>
                          <td className="px-4 py-2 text-right text-amber-800 tabular-nums dark:text-amber-300">
                            {formatCurrencyBRL(batch.charged_cents)}
                          </td>
                          <td
                            className={`px-4 py-2 text-right font-medium tabular-nums ${moneyTone(batch.profit_cents)}`}
                          >
                            {formatCurrencyBRL(batch.profit_cents)}
                          </td>
                          <td
                            className={`px-4 py-2 text-right tabular-nums ${moneyTone(batch.redemption_fee_saved_cents)}`}
                          >
                            {batch.redemption_fee_saved_cents === 0
                              ? "—"
                              : formatCurrencyBRL(
                                  batch.redemption_fee_saved_cents,
                                )}
                          </td>
                          {/* Points, not money: no colour, and the grid's own grammar. */}
                          <td className="px-4 py-2 text-right text-slate-600 tabular-nums dark:text-slate-300">
                            {formatPoints(batch.remaining_points)}
                          </td>
                          {/* The row shows what is actually transferred; the parts are in the
                          opened batch below. */}
                          <td className="px-4 py-2">
                            <SettlementList
                              settlements={batch.total_settlements}
                            />
                          </td>
                        </tr>
                        {open && (
                          <tr>
                            <td colSpan={6} className="p-0">
                              <BatchLines batch={batch} />
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    )
                  })
                )}
              </tbody>
            </table>
          </div>

          {(entry.total_settlements.length > 0 ||
            entry.fee_settlements.length > 0) && (
            <footer className="flex flex-col gap-1 border-t bg-amber-50/60 px-4 py-3 text-base dark:bg-amber-950/20">
              <p className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                Acerto do período
              </p>
              <SettlementCalculation
                agencies={entry.agencies}
                parts={entry.parts}
                settlements={entry.total_settlements}
              />
            </footer>
          )}
        </section>
      ))}

      {showTotals && (
        <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
          <header className="bg-[#1F3864] px-4 py-2.5 font-semibold text-white">
            Consolidado
          </header>
          <div className="grid grid-cols-2 divide-x divide-slate-200 bg-slate-50 dark:divide-slate-700 dark:bg-slate-900/40">
            <div className="px-4 py-3">
              <p className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                Custo
              </p>
              <p className="text-2xl font-semibold text-amber-800 tabular-nums dark:text-amber-300">
                {formatCurrencyBRL(closing.totals.cost_cents)}
              </p>
            </div>
            <div className="px-4 py-3">
              <p className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                Lucro
              </p>
              <p
                className={`text-2xl font-semibold tabular-nums ${moneyTone(closing.totals.profit_cents)}`}
              >
                {formatCurrencyBRL(closing.totals.profit_cents)}
              </p>
            </div>
          </div>
          {(closing.totals.total_settlements.length > 0 ||
            closing.totals.fee_settlements.length > 0) && (
            <footer className="flex flex-col gap-1 border-t bg-amber-50/60 px-4 py-3 text-base dark:bg-amber-950/20">
              <SettlementCalculation
                agencies={closing.totals.agencies}
                parts={closing.totals.parts}
                settlements={closing.totals.total_settlements}
              />
            </footer>
          )}
        </section>
      )}
    </div>
  )
}
