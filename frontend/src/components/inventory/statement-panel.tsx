"use client"

import { useCallback, useEffect, useMemo, useRef, useState, useTransition } from "react"
import { toast } from "sonner"
import { AlertTriangle, Check, FileUp, Loader2, Plug, RefreshCw, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { applyPaddedThousandsMask } from "@/lib/points"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import {
  commitAzulStatementAction,
  commitStatementAction,
  connectAzulAccountAction,
  getAzulStatementAction,
  previewStatementAction,
} from "@/actions/statement"
import type { Agency } from "@/schemas/agency"
import type {
  AzulSyncState,
  AzulSyncStateKind,
  PointBatchOrigin,
  StatementBatchSelection,
  StatementCreditLine,
  StatementReconciliation,
  StatementVerdict,
} from "@/schemas/inventory"

// Which reconciliation each program has. Azul is synced (no upload); Smiles still uploads its
// HTML. Every other program has no statement path, so the panel says so rather than showing
// controls that lead nowhere.
export type StatementProgram = "azul" | "smiles" | "other"

interface Props {
  accountId: string
  program: StatementProgram
  programName: string
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

// A full timestamp (dd/mm/yyyy HH:MM) for the sync badge, which reports a moment rather than a
// day. Returns "" for a null/absent reading so the caller can leave it out.
function formatDateTime(value: string | null | undefined): string {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return date.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
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

// The outcome both commit paths return, so ReconciliationResult can toast the same way whether
// the credits came from an uploaded Smiles file or the synced Azul window.
type CommitOutcome = { created: number; credits: number; skipped: number }

export function StatementPanel({ accountId, program, programName, origins, agencies }: Props) {
  if (program === "azul") {
    return <AzulStatementPanel accountId={accountId} origins={origins} agencies={agencies} />
  }
  if (program === "smiles") {
    return <SmilesStatementPanel accountId={accountId} origins={origins} agencies={agencies} />
  }
  return (
    <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
      <header className="flex flex-wrap items-center gap-3 bg-[#1F3864] px-4 py-2.5 text-white">
        <FileUp className="size-4" />
        <h2 className="font-semibold">Conciliar extrato</h2>
      </header>
      <p className="p-4 text-sm text-slate-500">
        A conciliação de extrato existe hoje só para Azul e Smiles. A conta selecionada é do
        programa {programName}.
      </p>
    </section>
  )
}

// --- Smiles: the upload path, unchanged in substance ---

function SmilesStatementPanel({
  accountId,
  origins,
  agencies,
}: {
  accountId: string
  origins: PointBatchOrigin[]
  agencies: Agency[]
}) {
  const [pending, startTransition] = useTransition()
  const [file, setFile] = useState<File | null>(null)
  const [result, setResult] = useState<StatementReconciliation | null>(null)
  const fileName = useRef<string>("")

  function preview() {
    if (!file) {
      toast.error("Escolha o arquivo do extrato.")
      return
    }
    const form = new FormData()
    form.set("file", file)
    form.set("account_id", accountId)
    form.set("format", "smiles")
    startTransition(async () => {
      const response = await previewStatementAction(form)
      if ("error" in response) {
        toast.error(response.error)
        return
      }
      setResult(response.reconciliation)
    })
  }

  // The Smiles commit re-sends the file: the statement is read and reconciled again from
  // scratch on the server, so the selection alone is not enough.
  async function commit(selections: StatementBatchSelection[]): Promise<CommitOutcome | null> {
    if (!file) {
      toast.error("Escolha o arquivo do extrato.")
      return null
    }
    const form = new FormData()
    form.set("file", file)
    form.set("account_id", accountId)
    form.set("format", "smiles")
    form.set("batches", JSON.stringify(selections))

    const response = await commitStatementAction(form)
    if ("error" in response) {
      toast.error(response.error)
      return null
    }
    announceCommit(response)
    // Reconcile again so what was just recorded shows as settled, rather than leaving the
    // screen offering to create it a second time.
    const again = await previewStatementAction(form)
    if (!("error" in again)) setResult(again.reconciliation)
    return response
  }

  return (
    <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
      <header className="flex flex-wrap items-center gap-3 bg-[#1F3864] px-4 py-2.5 text-white">
        <FileUp className="size-4" />
        <h2 className="font-semibold">Conciliar extrato da Smiles</h2>
        <span className="text-xs text-white/70">
          Nada do arquivo é guardado — ele é lido, comparado e descartado.
        </span>
      </header>

      <div className="flex flex-wrap items-end gap-3 border-b bg-slate-50 px-4 py-3 dark:bg-slate-900/40">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="statement-file" className="text-xs text-slate-500">
            Arquivo do extrato (.html)
          </Label>
          <Input
            id="statement-file"
            type="file"
            accept=".html,text/html"
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
        <ReconciliationResult
          result={result}
          origins={origins}
          agencies={agencies}
          pending={pending}
          onCommit={(selections) =>
            new Promise((resolve) => startTransition(async () => resolve(await commit(selections))))
          }
        />
      )}
    </section>
  )
}

// --- Azul: the synced path, no upload ---

const AZUL_BADGE: Record<
  AzulSyncStateKind,
  { label: string; variant: "default" | "secondary" | "destructive" | "outline"; className: string }
> = {
  pending: { label: "Aguardando sincronização", variant: "secondary", className: "" },
  ok: {
    label: "Sincronizado",
    variant: "outline",
    className: "border-emerald-300 text-emerald-700 dark:border-emerald-800 dark:text-emerald-400",
  },
  error: {
    label: "Erro na última tentativa",
    variant: "outline",
    className: "border-amber-300 text-amber-800 dark:border-amber-800 dark:text-amber-300",
  },
  inconsistent: {
    label: "Saldo não bateu",
    variant: "outline",
    className: "border-red-300 text-red-700 dark:border-red-800 dark:text-red-400",
  },
  needs_attention: { label: "Precisa de atenção", variant: "destructive", className: "" },
}

function AzulStatementPanel({
  accountId,
  origins,
  agencies,
}: {
  accountId: string
  origins: PointBatchOrigin[]
  agencies: Agency[]
}) {
  const [pending, startTransition] = useTransition()
  const [loaded, setLoaded] = useState(false)
  const [syncState, setSyncState] = useState<AzulSyncState | null>(null)
  const [result, setResult] = useState<StatementReconciliation | null>(null)

  // Quiet refetch used by the connect poll and after a commit: it refreshes the state and window
  // without touching `loaded` or the "Atualizar" spinner, so the badge updates in place.
  const refresh = useCallback(async () => {
    const response = await getAzulStatementAction(accountId)
    if ("error" in response) return
    setSyncState(response.statement.sync_state)
    setResult(response.statement.reconciliation)
  }, [accountId])

  function load() {
    startTransition(async () => {
      const response = await getAzulStatementAction(accountId)
      setLoaded(true)
      if ("error" in response) {
        toast.error(response.error)
        return
      }
      setSyncState(response.statement.sync_state)
      setResult(response.statement.reconciliation)
    })
  }

  // Read the state and the synced window once when the account changes. Passing load by
  // reference (not an inline closure) scopes the effect to accountId alone.
  useEffect(load, [accountId])

  const connecting = syncState?.connecting ?? false

  // The connect is asynchronous: the server logs in for minutes in the background, so while it is
  // in flight, poll the state until it clears (then Connected is true, or the state carries the
  // failure). Also covers a page reopened mid-connect, since `connecting` comes from the server.
  useEffect(() => {
    if (!connecting) return
    const interval = setInterval(() => {
      void refresh()
    }, 4000)
    return () => clearInterval(interval)
  }, [connecting, refresh])

  // Announce the outcome once, on the transition out of connecting - success or the failure detail.
  const wasConnecting = useRef(false)
  useEffect(() => {
    if (wasConnecting.current && !connecting) {
      if (syncState?.connected) {
        toast.success("Conta conectada. A primeira sincronização acontece na próxima rodada.")
      } else {
        toast.error(
          syncState?.detail
            ? `Não consegui conectar: ${syncState.detail}`
            : "Não consegui conectar a conta. Tente novamente.",
        )
      }
    }
    wasConnecting.current = connecting
  }, [connecting, syncState?.connected, syncState?.detail])

  function connect() {
    startTransition(async () => {
      const response = await connectAzulAccountAction(accountId)
      if ("error" in response) {
        toast.error(response.error)
        return
      }
      toast.info("Login iniciado. Isso pode levar alguns minutos…")
      // Pick up connecting=true right away; the poll effect drives it from here.
      await refresh()
    })
  }

  async function commit(selections: StatementBatchSelection[]): Promise<CommitOutcome | null> {
    const response = await commitAzulStatementAction(accountId, selections)
    if ("error" in response) {
      toast.error(response.error)
      return null
    }
    announceCommit(response)
    await refresh()
    return response
  }

  const connected = syncState?.connected ?? false
  const hasWindow = result !== null && result.from !== ""

  return (
    <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
      <header className="flex flex-wrap items-center gap-3 bg-[#1F3864] px-4 py-2.5 text-white">
        <RefreshCw className="size-4" />
        <h2 className="font-semibold">Extrato da Azul (sincronizado)</h2>
        <span className="text-xs text-white/70">
          O extrato é lido sozinho da Azul — não há arquivo para subir.
        </span>
        {loaded && (
          <Button
            variant="ghost"
            size="sm"
            onClick={load}
            disabled={pending}
            className="ml-auto text-white hover:bg-white/10 hover:text-white"
          >
            {pending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            Atualizar
          </Button>
        )}
      </header>

      {!loaded ? (
        <div className="flex items-center gap-2 p-4 text-sm text-slate-500">
          <Loader2 className="size-4 animate-spin" />
          Lendo o estado da sincronização…
        </div>
      ) : connecting && !connected ? (
        <div className="flex items-center gap-2 p-4 text-sm text-slate-600 dark:text-slate-300">
          <Loader2 className="size-4 animate-spin" />
          Conectando à Azul… o login roda em segundo plano e pode levar alguns minutos. Pode deixar
          esta tela aberta.
        </div>
      ) : !connected ? (
        <div className="flex flex-col items-start gap-3 p-4">
          <p className="text-sm text-slate-600 dark:text-slate-300">
            Esta conta ainda não foi conectada. Conectar loga na Azul uma vez, com as credenciais
            já salvas na conta, para descobrir a chave que a sincronização de leitura usa. Depois
            disso o extrato é lido sozinho a cada rodada.
          </p>
          {syncState?.detail && syncState.state !== "pending" && (
            <p className="text-xs text-amber-800 dark:text-amber-300">
              Última tentativa: {syncState.detail}
            </p>
          )}
          <Button onClick={connect} disabled={pending}>
            {pending ? <Loader2 className="size-4 animate-spin" /> : <Plug className="size-4" />}
            Conectar conta Azul
          </Button>
        </div>
      ) : (
        <div className="flex flex-col gap-4 p-4">
          {syncState && (
            <SyncBadge state={syncState} onReconnect={connect} reconnecting={pending || connecting} />
          )}

          {!hasWindow ? (
            <p className="text-sm text-slate-500">
              Conta conectada. Ainda não há uma janela de extrato sincronizada — ela aparece após a
              primeira leitura em que o saldo mudar.
            </p>
          ) : (
            <ReconciliationResult
              result={result}
              origins={origins}
              agencies={agencies}
              pending={pending}
              hideAccountNumber
              onCommit={(selections) =>
                new Promise((resolve) =>
                  startTransition(async () => resolve(await commit(selections))),
                )
              }
            />
          )}
        </div>
      )}
    </section>
  )
}

function SyncBadge({
  state,
  onReconnect,
  reconnecting,
}: {
  state: AzulSyncState
  onReconnect: () => void
  reconnecting: boolean
}) {
  const badge = AZUL_BADGE[state.state]
  const lastSynced = formatDateTime(state.last_synced_at)

  return (
    <div className="flex flex-col gap-2 rounded-md border bg-slate-50 p-3 dark:bg-slate-900/40">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm">
        <Badge variant={badge.variant} className={badge.className}>
          {badge.label}
        </Badge>
        {lastSynced && (
          <span className="text-slate-500">
            Última sincronização:{" "}
            <span className="font-medium text-slate-700 dark:text-slate-200">{lastSynced}</span>
          </span>
        )}
        {state.last_balance_points != null && (
          <span className="text-slate-500">
            Saldo lido:{" "}
            <span className="font-medium tabular-nums text-slate-700 dark:text-slate-200">
              {formatPoints(state.last_balance_points)}
            </span>
          </span>
        )}
        {/* Reconnecting is the manual escape hatch: re-run the login to refresh the stored key,
            which is what an account raised for attention needs after its password was fixed. */}
        <Button
          variant="ghost"
          size="sm"
          onClick={onReconnect}
          disabled={reconnecting}
          className="ml-auto"
        >
          {reconnecting ? <Loader2 className="size-4 animate-spin" /> : <Plug className="size-4" />}
          Reconectar
        </Button>
      </div>

      {state.state === "needs_attention" && (
        <p className="text-xs text-red-700 dark:text-red-400">
          O login falhou vezes demais seguidas — confira o login e a senha salvos na conta e
          reconecte.
        </p>
      )}
      {state.state === "inconsistent" && (
        <p className="text-xs text-red-700 dark:text-red-400">
          O saldo da Azul não bateu com a soma do extrato, então a janela não foi atualizada. A
          comparação abaixo é a última que fechou.
        </p>
      )}
      {state.detail && state.state !== "ok" && state.state !== "pending" && (
        <p className="text-xs text-slate-500">{state.detail}</p>
      )}
    </div>
  )
}

// Shared toast for both commit paths: created counts batches, not credits, because credits of
// the same origin and payer are summed into one batch.
function announceCommit(response: CommitOutcome) {
  const lots = `${response.created} ${response.created === 1 ? "lote lançado" : "lotes lançados"}`
  const from = `a partir de ${response.credits} ${response.credits === 1 ? "crédito" : "créditos"}`
  toast.success(
    response.skipped > 0
      ? `${lots} ${from}. ${response.skipped} já existiam.`
      : `${lots} ${from}.`,
  )
}

// --- Shared reconciliation view ---

// Everything below the source-specific controls: the balance check, the credits-without-a-batch
// table with its bulk bar, and the booking/refund comparisons. It owns the operator's draft
// selections and resets them whenever a fresh reconciliation arrives (a new preview or refetch).
function ReconciliationResult({
  result,
  origins,
  agencies,
  pending,
  hideAccountNumber,
  onCommit,
}: {
  result: StatementReconciliation
  origins: PointBatchOrigin[]
  agencies: Agency[]
  pending: boolean
  hideAccountNumber?: boolean
  onCommit: (selections: StatementBatchSelection[]) => Promise<CommitOutcome | null>
}) {
  const [drafts, setDrafts] = useState<Record<number, CreditDraft>>({})
  const [selected, setSelected] = useState<Set<number>>(new Set())
  // What the bulk bar holds. A month brings a dozen credits of the same origin and agency,
  // so filling them one by one is the difference between three clicks and sixty.
  const [bulk, setBulk] = useState<CreditDraft>(BLANK)

  // A fresh reconciliation (a new preview, or a refetch after a commit) invalidates every draft:
  // the credit indexes it numbered belong to that reading, not this one. Resetting during render
  // when the prop identity changes is React's recommended alternative to an effect for this - it
  // avoids the extra commit-and-rerender an effect would cost. See react.dev "You Might Not Need
  // an Effect".
  const [seenResult, setSeenResult] = useState(result)
  if (seenResult !== result) {
    setSeenResult(result)
    setDrafts({})
    setSelected(new Set())
    setBulk(BLANK)
  }

  const missingCredits = useMemo(
    () => result.credits.filter((credit) => credit.verdict === "missing_in_panel"),
    [result],
  )
  const settledCredits = useMemo(
    () => result.credits.filter((credit) => credit.verdict !== "missing_in_panel"),
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
  const plannedBatches = useMemo(() => {
    const keys = new Set(
      ready.map(
        (index) =>
          `${drafts[index].originId}|${drafts[index].fundingAgencyId}|${drafts[index].note}`,
      ),
    )
    return keys.size
  }, [ready, drafts])

  function commit() {
    if (ready.length === 0) return
    const selections = ready.map((index) => {
      const draft = drafts[index]
      return {
        index,
        origin_id: draft.originId,
        // Blank reads as zero rather than as an error: plenty of credits are free.
        cost_cents: draft.cost.trim() === "" ? 0 : (parseCurrencyBRLInput(draft.cost) ?? 0),
        funding_agency_id: draft.fundingAgencyId,
        note: draft.note.trim(),
      }
    })
    void onCommit(selections)
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

  const balanceAgrees = result.statement_net_points === result.panel_net_points
  const allSelected = missingCredits.length > 0 && selected.size === missingCredits.length

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm">
        <span className="text-slate-500">
          Período do extrato:{" "}
          <span className="font-medium text-slate-700 dark:text-slate-200">
            {formatDate(result.from)} a {formatDate(result.to)}
          </span>
        </span>
        {!hideAccountNumber && result.account_number && (
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
              onValueChange={(value) => setBulk((current) => ({ ...current, fundingAgencyId: value }))}
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
