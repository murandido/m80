"use client"

import { useMemo, useRef, useState, useTransition } from "react"
import { toast } from "sonner"
import { AlertTriangle, Check, FileUp, Loader2, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { applyPaddedThousandsMask } from "@/lib/points"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { commitStatementAction, previewStatementAction } from "@/actions/statement"
import type { Agency } from "@/schemas/agency"
import type {
  PointBatchOrigin,
  StatementCreditLine,
  StatementReconciliation,
  StatementVerdict,
} from "@/schemas/inventory"

interface Props {
  accountId: string
  origins: PointBatchOrigin[]
  agencies: Agency[]
}

function formatPoints(points: number): string {
  const sign = points < 0 ? "-" : ""
  return sign + applyPaddedThousandsMask(String(Math.abs(points)))
}

function formatDate(value: string): string {
  return value ? value.split("-").reverse().join("/") : ""
}

// What each verdict says, in the operator's terms: what is wrong, not which side lacks a
// row. Colour goes on what has to be judged, the same rule the closing panel follows.
const VERDICTS: Record<StatementVerdict, { label: string; className: string }> = {
  matched: { label: "Confere", className: "text-emerald-700 dark:text-emerald-400" },
  missing_in_panel: { label: "Falta no painel", className: "text-amber-800 dark:text-amber-300" },
  missing_in_statement: { label: "Falta no extrato", className: "text-amber-800 dark:text-amber-300" },
  amount_differs: { label: "Valor diferente", className: "text-red-700 dark:text-red-400" },
}

// What the operator supplies for a credit: the things a statement cannot know.
interface CreditDraft {
  originId: string
  cost: string
  fundingAgencyId: string
  // note is both the batch's label and the way to keep two acquisitions apart. Credits are
  // grouped by origin, payer and this: a statement carrying two Inter purchases a week
  // apart would otherwise be summed into one batch at a blended rate that is neither.
  note: string
}

const BLANK: CreditDraft = { originId: "", cost: "", fundingAgencyId: "", note: "" }

export function StatementPanel({ accountId, origins, agencies }: Props) {
  const [pending, startTransition] = useTransition()
  const [format, setFormat] = useState<"azul" | "smiles">("azul")
  const [file, setFile] = useState<File | null>(null)
  const [result, setResult] = useState<StatementReconciliation | null>(null)
  const [drafts, setDrafts] = useState<Record<number, CreditDraft>>({})
  const [selected, setSelected] = useState<Set<number>>(new Set())
  // What the bulk bar holds. A month brings a dozen credits of the same origin and agency,
  // so filling them one by one is the difference between three clicks and sixty.
  const [bulk, setBulk] = useState<CreditDraft>(BLANK)
  const fileName = useRef<string>("")

  const missingCredits = useMemo(
    () => result?.credits.filter((credit) => credit.verdict === "missing_in_panel") ?? [],
    [result],
  )
  const settledCredits = useMemo(
    () => result?.credits.filter((credit) => credit.verdict !== "missing_in_panel") ?? [],
    [result],
  )

  // A selection is ready when it has an origin and an agency. Cost may be blank: a club
  // subscription and flight accrual both arrive for nothing.
  const ready = useMemo(
    () =>
      [...selected].filter((index) => {
        const draft = drafts[index]
        return draft?.originId && draft?.fundingAgencyId
      }),
    [selected, drafts],
  )

  const bulkCost = useMemo(
    () => (bulk.cost.trim() === "" ? null : parseCurrencyBRLInput(bulk.cost)),
    [bulk.cost],
  )

  // The money the commit is about to record, added up across the marked credits - the
  // figure to check before pressing, and the one a mistyped separator shows up in.
  const plannedCents = useMemo(
    () =>
      ready.reduce((total, index) => {
        const typed = drafts[index].cost
        return total + (typed.trim() === "" ? 0 : (parseCurrencyBRLInput(typed) ?? 0))
      }, 0),
    [ready, drafts],
  )

  // What will actually be written: credits of the same origin and payer are summed into a
  // single batch, because a purchase that arrived in four instalments is one acquisition.
  // Saying so before the button is pressed is what keeps "lancei 24 e apareceram 2" from
  // reading as a bug.
  const plannedBatches = useMemo(() => {
    const keys = new Set(
      ready.map(
        (index) =>
          `${drafts[index].originId}|${drafts[index].fundingAgencyId}|${drafts[index].note}`,
      ),
    )
    return keys.size
  }, [ready, drafts])

  function buildForm(): FormData | null {
    if (!file) {
      toast.error("Escolha o arquivo do extrato.")
      return null
    }
    const form = new FormData()
    form.set("file", file)
    form.set("account_id", accountId)
    form.set("format", format)
    return form
  }

  function preview() {
    const form = buildForm()
    if (!form) return
    startTransition(async () => {
      const response = await previewStatementAction(form)
      if ("error" in response) {
        toast.error(response.error)
        return
      }
      setResult(response.reconciliation)
      setDrafts({})
      setSelected(new Set())
    })
  }

  function commit() {
    const form = buildForm()
    if (!form) return
    if (ready.length === 0) return

    form.set(
      "batches",
      JSON.stringify(
        ready.map((index) => {
          const draft = drafts[index]
          return {
            index,
            origin_id: draft.originId,
            // Blank reads as zero rather than as an error: plenty of credits are free.
            cost_cents: draft.cost.trim() === "" ? 0 : (parseCurrencyBRLInput(draft.cost) ?? 0),
            funding_agency_id: draft.fundingAgencyId,
            note: draft.note.trim(),
          }
        }),
      ),
    )

    startTransition(async () => {
      const response = await commitStatementAction(form)
      if ("error" in response) {
        toast.error(response.error)
        return
      }
      const lots = `${response.created} ${response.created === 1 ? "lote lançado" : "lotes lançados"}`
      const from = `a partir de ${response.credits} ${response.credits === 1 ? "crédito" : "créditos"}`
      toast.success(
        response.skipped > 0
          ? `${lots} ${from}. ${response.skipped} já existiam.`
          : `${lots} ${from}.`,
      )
      // Reconcile again so what was just recorded shows as settled, rather than leaving the
      // screen offering to create it a second time.
      const again = await previewStatementAction(form)
      if (!("error" in again)) setResult(again.reconciliation)
      setDrafts({})
      setSelected(new Set())
    })
  }

  function setDraft(index: number, changes: Partial<CreditDraft>) {
    setDrafts((current) => ({ ...current, [index]: { ...BLANK, ...current[index], ...changes } }))
    // Touching a row is saying you want it: having to tick the box as well would be a
    // second gesture for a decision already made.
    setSelected((current) => new Set(current).add(index))
  }

  function toggle(index: number) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }

  function toggleAll() {
    setSelected((current) =>
      current.size === missingCredits.length
        ? new Set()
        : new Set(missingCredits.map((credit) => credit.index)),
    )
  }

  // Applies the bulk bar to every selected row, leaving blank fields alone - so you can
  // set the agency for all of them and still give two of them a different origin.
  function applyBulk() {
    if (selected.size === 0) {
      toast.error("Marque os créditos antes de aplicar.")
      return
    }
    setDrafts((current) => {
      const next = { ...current }
      for (const index of selected) {
        next[index] = {
          ...BLANK,
          ...next[index],
          ...(bulk.originId ? { originId: bulk.originId } : {}),
          ...(bulk.cost.trim() !== "" ? { cost: bulk.cost } : {}),
          ...(bulk.fundingAgencyId ? { fundingAgencyId: bulk.fundingAgencyId } : {}),
          ...(bulk.note.trim() !== "" ? { note: bulk.note } : {}),
        }
      }
      return next
    })
  }

  const balanceAgrees = result ? result.statement_net_points === result.panel_net_points : true
  const allSelected = missingCredits.length > 0 && selected.size === missingCredits.length

  return (
    <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
      <header className="flex flex-wrap items-center gap-3 bg-[#1F3864] px-4 py-2.5 text-white">
        <FileUp className="size-4" />
        <h2 className="font-semibold">Conciliar extrato</h2>
        <span className="text-xs text-white/70">
          Nada do arquivo é guardado — ele é lido, comparado e descartado.
        </span>
      </header>

      <div className="flex flex-wrap items-end gap-3 border-b bg-slate-50 px-4 py-3 dark:bg-slate-900/40">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="statement-format" className="text-xs text-slate-500">
            Programa
          </Label>
          <Select
            value={format}
            onValueChange={(value) => {
              setFormat(value as "azul" | "smiles")
              // The old comparison describes the other programme's file; keeping it on
              // screen next to a new choice would read as if it applied.
              setResult(null)
            }}
          >
            <SelectTrigger id="statement-format" className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="azul">Azul (.csv)</SelectItem>
              <SelectItem value="smiles">Smiles (.html)</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="statement-file" className="text-xs text-slate-500">
            Arquivo do extrato
          </Label>
          <Input
            id="statement-file"
            type="file"
            accept={format === "azul" ? ".csv,text/csv" : ".html,text/html"}
            onChange={(event) => {
              const chosen = event.target.files?.[0] ?? null
              setFile(chosen)
              fileName.current = chosen?.name ?? ""
              setResult(null)
            }}
            className="w-72"
          />
        </div>

        <Button onClick={preview} disabled={pending || !file}>
          {pending && <Loader2 className="size-4 animate-spin" />}
          Conciliar
        </Button>
      </div>

      {result && (
        <div className="flex flex-col gap-6 p-4">
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm">
            <span className="text-slate-500">
              Período do extrato:{" "}
              <span className="font-medium text-slate-700 dark:text-slate-200">
                {formatDate(result.from)} a {formatDate(result.to)}
              </span>
            </span>
            {result.account_number && (
              <span className="text-slate-500">Conta na companhia: {result.account_number}</span>
            )}
          </div>

          {balanceAgrees ? (
            <p className="flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-400">
              <Check className="size-4" />
              A variação de saldo do extrato bate com a do painel: {formatPoints(result.statement_net_points)} pontos.
            </p>
          ) : (
            <div className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-800 dark:bg-amber-950/40">
              <X className="mt-0.5 size-4 shrink-0 text-amber-700 dark:text-amber-400" />
              <p className="text-amber-900 dark:text-amber-200">
                A variação de saldo não bate: o extrato diz{" "}
                <strong>{formatPoints(result.statement_net_points)}</strong> e o painel diz{" "}
                <strong>{formatPoints(result.panel_net_points)}</strong>. A diferença está nas listas
                abaixo — resolva o que estiver marcado e os dois números se encontram.
              </p>
            </div>
          )}

          {missingCredits.length > 0 && (
            <div className="flex flex-col gap-3">
              <div>
                <h3 className="flex items-center gap-2 text-sm font-semibold">
                  <AlertTriangle className="size-4 text-amber-700 dark:text-amber-400" />
                  Créditos sem lote ({missingCredits.length})
                </h3>
                <p className="mt-1 text-xs text-slate-500">
                  Pontos que entraram na conta e nunca foram precificados. Marque, informe o custo e
                  a origem, e lance — o custeio recalcula sozinho depois. Créditos com a mesma origem
                  e a mesma agência viram um lote só, somados e datados no mais antigo deles. Se o
                  extrato trouxer duas compras do mesmo parceiro que não são a mesma aquisição, dê
                  rótulos diferentes a elas — rótulo diferente, lote separado.
                </p>
              </div>

              {/* One month brings a dozen credits from the same partner, bought by the same
                  agency. Filling them one at a time is the difference between three clicks
                  and sixty. Blank fields are left alone, so you can set the agency for all
                  and still give two of them a different origin. */}
              <div className="flex flex-wrap items-end gap-2 rounded-md border bg-slate-50 p-3 dark:bg-slate-900/40">
                <span className="text-xs font-medium uppercase tracking-wide text-slate-500">
                  Preencher os marcados
                </span>
                <Select
                  value={bulk.originId}
                  onValueChange={(value) => setBulk((current) => ({ ...current, originId: value }))}
                >
                  <SelectTrigger className="w-44">
                    <SelectValue placeholder="Origem" />
                  </SelectTrigger>
                  <SelectContent>
                    {origins.map((origin) => (
                      <SelectItem key={origin.id} value={origin.id}>
                        {origin.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {/* A dot and a comma both separate the decimals, so "1.500" is a real and
                    a half rather than fifteen hundred. This bar writes the same cost onto
                    every marked credit, so it is the one place worth spelling out. */}
                <div className="flex w-32 flex-col gap-0.5">
                  <Input
                    inputMode="decimal"
                    placeholder="Custo (R$)"
                    value={bulk.cost}
                    onChange={(event) => setBulk((current) => ({ ...current, cost: event.target.value }))}
                  />
                  {bulkCost !== null && (
                    <span className="text-xs text-slate-500">= {formatCurrencyBRL(bulkCost)}</span>
                  )}
                </div>
                <Select
                  value={bulk.fundingAgencyId}
                  onValueChange={(value) =>
                    setBulk((current) => ({ ...current, fundingAgencyId: value }))
                  }
                >
                  <SelectTrigger className="w-44">
                    <SelectValue placeholder="Bancado por" />
                  </SelectTrigger>
                  <SelectContent>
                    {agencies.map((agency) => (
                      <SelectItem key={agency.id} value={agency.id}>
                        {agency.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Input
                  placeholder="Rótulo (opcional)"
                  value={bulk.note}
                  onChange={(event) => setBulk((current) => ({ ...current, note: event.target.value }))}
                  className="w-40"
                />
                <Button variant="secondary" onClick={applyBulk} disabled={pending}>
                  Aplicar aos {selected.size} marcados
                </Button>
              </div>

              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b bg-slate-100 text-xs uppercase tracking-wide text-slate-600 dark:bg-slate-800 dark:text-slate-300">
                      <th className="w-10 px-3 py-2">
                        <input
                          type="checkbox"
                          checked={allSelected}
                          onChange={toggleAll}
                          className="size-4 accent-slate-800"
                          aria-label={allSelected ? "Desmarcar todos" : "Marcar todos"}
                        />
                      </th>
                      <th className="px-3 py-2 text-left font-medium">Data</th>
                      <th className="px-3 py-2 text-right font-medium">Qnt. de Pontos</th>
                      <th className="px-3 py-2 text-left font-medium">Descrição</th>
                      <th className="px-3 py-2 text-left font-medium">Origem</th>
                      <th className="px-3 py-2 text-left font-medium">Custo (R$)</th>
                      <th className="px-3 py-2 text-left font-medium">Bancado por</th>
                      <th className="px-3 py-2 text-left font-medium">Rótulo</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                    {missingCredits.map((credit) => (
                      <CreditRow
                        key={credit.index}
                        credit={credit}
                        draft={drafts[credit.index]}
                        checked={selected.has(credit.index)}
                        origins={origins}
                        agencies={agencies}
                        onToggle={() => toggle(credit.index)}
                        onChange={(changes) => setDraft(credit.index, changes)}
                      />
                    ))}
                  </tbody>
                </table>
              </div>

              <div className="flex items-center gap-3">
                <Button onClick={commit} disabled={pending || ready.length === 0}>
                  {pending && <Loader2 className="size-4 animate-spin" />}
                  {ready.length === 0
                    ? "Lançar créditos"
                    : `Lançar ${plannedBatches} ${plannedBatches === 1 ? "lote" : "lotes"}`}
                </Button>
                {ready.length > 0 && (
                  <span className="text-xs text-slate-500">
                    {ready.length > plannedBatches && (
                      <>
                        {ready.length} créditos somados em {plannedBatches}{" "}
                        {plannedBatches === 1 ? "lote" : "lotes"}.{" "}
                      </>
                    )}
                    Custo total: {formatCurrencyBRL(plannedCents)}.
                  </span>
                )}
                {selected.size > ready.length && (
                  <span className="text-xs text-amber-800 dark:text-amber-300">
                    {selected.size - ready.length} marcados ainda sem origem ou agência.
                  </span>
                )}
              </div>
            </div>
          )}

          <Comparison
            title="Emissões"
            empty="Nenhuma emissão no período."
            rows={result.bookings.map((line) => ({
              key: line.locator,
              label: line.locator,
              detail: formatDate(line.occurred_on),
              statement: line.statement_points,
              panel: line.panel_points,
              verdict: line.verdict,
            }))}
          />

          <Comparison
            title="Reembolsos"
            empty="Nenhum reembolso no período."
            rows={result.refunds.map((line) => ({
              key: line.locator,
              label: line.locator,
              detail: "",
              statement: line.statement_points,
              panel: line.panel_points,
              verdict: line.verdict,
            }))}
          />

          {settledCredits.length > 0 && (
            <Comparison
              title="Créditos já lançados"
              empty=""
              rows={settledCredits.map((credit) => ({
                key: `${credit.index}-${credit.batch_id}`,
                label: formatDate(credit.occurred_on),
                detail: credit.description,
                statement: credit.points,
                panel: credit.points,
                verdict: credit.verdict,
              }))}
            />
          )}
        </div>
      )}
    </section>
  )
}

function CreditRow({
  credit,
  draft,
  checked,
  origins,
  agencies,
  onToggle,
  onChange,
}: {
  credit: StatementCreditLine
  draft?: CreditDraft
  checked: boolean
  origins: PointBatchOrigin[]
  agencies: Agency[]
  onToggle: () => void
  onChange: (changes: Partial<CreditDraft>) => void
}) {
  return (
    <tr className={checked ? "bg-slate-50 dark:bg-slate-900/40" : undefined}>
      <td className="px-3 py-2">
        <input
          type="checkbox"
          checked={checked}
          onChange={onToggle}
          className="size-4 accent-slate-800"
          aria-label={`Marcar crédito de ${formatPoints(credit.points)} pontos`}
        />
      </td>
      <td className="px-3 py-2 tabular-nums">{formatDate(credit.occurred_on)}</td>
      <td className="px-3 py-2 text-right tabular-nums">{formatPoints(credit.points)}</td>
      <td className="max-w-xs truncate px-3 py-2 text-slate-500" title={credit.description}>
        {credit.description || "—"}
      </td>
      <td className="px-3 py-2">
        <Select value={draft?.originId ?? ""} onValueChange={(value) => onChange({ originId: value })}>
          <SelectTrigger className="w-40">
            <SelectValue placeholder="Escolha" />
          </SelectTrigger>
          <SelectContent>
            {origins.map((origin) => (
              <SelectItem key={origin.id} value={origin.id}>
                {origin.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </td>
      <td className="px-3 py-2">
        <Input
          inputMode="decimal"
          placeholder="0,00"
          value={draft?.cost ?? ""}
          onChange={(event) => onChange({ cost: event.target.value })}
          className="w-28"
        />
      </td>
      <td className="px-3 py-2">
        <Select
          value={draft?.fundingAgencyId ?? ""}
          onValueChange={(value) => onChange({ fundingAgencyId: value })}
        >
          <SelectTrigger className="w-40">
            <SelectValue placeholder="Escolha" />
          </SelectTrigger>
          <SelectContent>
            {agencies.map((agency) => (
              <SelectItem key={agency.id} value={agency.id}>
                {agency.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </td>
      <td className="px-3 py-2">
        <Input
          placeholder="Opcional"
          value={draft?.note ?? ""}
          onChange={(event) => onChange({ note: event.target.value })}
          className="w-36"
        />
      </td>
    </tr>
  )
}

function Comparison({
  title,
  empty,
  rows,
}: {
  title: string
  empty: string
  rows: {
    key: string
    label: string
    detail: string
    statement: number
    panel: number
    verdict: StatementVerdict
  }[]
}) {
  // Everything that matched is folded into a count. A reconciliation of eighty bookings
  // where seventy-eight agree is a screen about the other two.
  const differing = rows.filter((row) => row.verdict !== "matched")
  const matched = rows.length - differing.length

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold">
        {title}
        {matched > 0 && (
          <span className="ml-2 text-xs font-normal text-emerald-700 dark:text-emerald-400">
            {matched} {matched === 1 ? "confere" : "conferem"}
          </span>
        )}
      </h3>
      {differing.length === 0 ? (
        <p className="text-xs text-slate-500">{rows.length === 0 ? empty : "Tudo confere."}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-slate-100 text-xs uppercase tracking-wide text-slate-600 dark:bg-slate-800 dark:text-slate-300">
                <th className="px-3 py-2 text-left font-medium">Localizador</th>
                <th className="px-3 py-2 text-right font-medium">No extrato</th>
                <th className="px-3 py-2 text-right font-medium">No painel</th>
                <th className="px-3 py-2 text-left font-medium">Situação</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {differing.map((row) => {
                const { label, className } = VERDICTS[row.verdict]
                return (
                  <tr key={row.key}>
                    <td className="px-3 py-2">
                      <span className="font-medium">{row.label}</span>
                      {/* No dash when there is no date: an empty cell says "not here"
                          without pretending to be a value. */}
                      {row.detail && <span className="ml-2 text-slate-500">{row.detail}</span>}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums">
                      {row.statement === 0 ? (
                        <span className="text-slate-400">—</span>
                      ) : (
                        formatPoints(row.statement)
                      )}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums">
                      {row.panel === 0 ? (
                        <span className="text-slate-400">—</span>
                      ) : (
                        formatPoints(row.panel)
                      )}
                    </td>
                    <td className={`px-3 py-2 text-xs font-medium ${className}`}>{label}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
