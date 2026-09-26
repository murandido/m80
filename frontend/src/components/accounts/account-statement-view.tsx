"use client"

import { useEffect, useMemo, useRef, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { formatDistanceToNow } from "date-fns"
import { ptBR } from "date-fns/locale"
import { toast } from "sonner"
import {
  ArrowDownCircle,
  ArrowUpCircle,
  Boxes,
  ChevronLeft,
  ChevronRight,
  CircleSlash,
  Download,
  Link2,
  Link2Off,
  Loader2,
  Package,
  RefreshCcw,
  Search,
  ShoppingCart,
  X,
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { DateRangeFilter } from "@/components/filters/date-range-filter"
import { cn } from "@/lib/utils"
import { LoyaltyProgramBadge } from "./loyalty-program-badge"
import { LoyaltyProgramSummary } from "@/schemas/account"
import {
  type StatementEntries,
  type SyncedProgram,
  type StatementEntry,
  type StatementLineKind,
  type StatementLinkedBatch,
  type StatementLinkedIssuance,
  type StatementSyncKind,
} from "@/schemas/inventory"
import {
  assignAzulCityCodeAction,
  getAccountBatchesAction,
  linkStatementBatchAction,
  linkStatementBookingAction,
  searchAccountIssuancesAction,
  unlinkStatementBatchAction,
  unlinkStatementBookingAction,
  type BatchLinkOption,
  type IssuanceLinkOption,
} from "@/actions/statement"
import { formatCurrencyBRL } from "@/lib/currency"

// The kinds, in the panel's Portuguese, with a colour and an icon each. Mirrors data.AzulLine*.
const KIND_META: Record<
  StatementLineKind,
  { label: string; className: string; icon: typeof ArrowDownCircle }
> = {
  redemption: { label: "Resgate", className: "text-rose-600", icon: ArrowUpCircle },
  refund: { label: "Reembolso", className: "text-emerald-600", icon: ArrowDownCircle },
  accrual: { label: "Acúmulo", className: "text-emerald-600", icon: ArrowDownCircle },
  purchase: { label: "Compra", className: "text-emerald-600", icon: ShoppingCart },
  expiration: { label: "Expiração", className: "text-slate-500", icon: CircleSlash },
  other: { label: "Outro", className: "text-slate-500", icon: CircleSlash },
}

// The sync-state badge, mirroring the labels used on the inventory panel.
const SYNC_STATE_META: Record<StatementSyncKind, { label: string; className: string }> = {
  pending: { label: "Aguardando 1ª sync", className: "bg-slate-100 text-slate-600" },
  ok: { label: "Sincronizado", className: "bg-emerald-100 text-emerald-700" },
  error: { label: "Erro na sincronização", className: "bg-rose-100 text-rose-700" },
  inconsistent: { label: "Saldo divergente", className: "bg-amber-100 text-amber-800" },
  needs_mfa: { label: "Precisa de código novo", className: "bg-amber-100 text-amber-800" },
  needs_attention: { label: "Precisa de atenção", className: "bg-rose-100 text-rose-700" },
}

// The airline's own English type descriptions, in Portuguese. The kind badge already gives the
// category; this adds the specific movement. A description not in the table (e.g. the already-
// Portuguese "Aceite de Campanha Promocional") is shown as-is.
const TYPE_LABELS: Record<string, string> = {
  "Direct redemption": "Resgate direto",
  "Airline Issuance": "Emissão Azul",
  "Redemption Refund": "Estorno de resgate",
  Sale: "Compra de pontos",
  "Points Corrections": "Correção de pontos",
}

function typeLabel(typeDescription: string | null | undefined): string {
  const raw = (typeDescription ?? "").trim()
  if (!raw) return ""
  return TYPE_LABELS[raw] ?? raw
}

// Which kinds carry a booking link. Only redemptions and refunds (cancellations) map to an
// issuance, by locator. Everything else either links to a batch or, like "other", to nothing.
function isLinkable(kind: StatementLineKind): boolean {
  return kind === "redemption" || kind === "refund"
}

// Point entries - purchases (which include campaign/promotion credits) and accruals - link to a
// batch (lote) rather than an issuance.
function isBatchLinkable(kind: StatementLineKind): boolean {
  return kind === "accrual" || kind === "purchase"
}

// The line's detail, composed from the structured fields rather than the single stored
// description (which was just the partner and read as "AZUL" on a promotion). The type label says
// what the movement is and the partner who it came from; the route is rendered separately so its
// cities can be made assignable. Each shown only when it adds something, never truncated.
function detailParts(entry: StatementEntry): string[] {
  const parts: string[] = []
  const label = typeLabel(entry.type_description)
  if (label) parts.push(label)
  // The note names what a non-flight redemption bought (the Azul Viagens hotel/product) - the
  // useful detail on a line that has no route.
  if (entry.note && entry.note.trim()) {
    parts.push(entry.note.trim())
  }
  // "Azul" as a partner adds nothing (it is the programme itself); a real partner names the
  // source of the points, which is the useful part of an accrual.
  if (entry.partner_name && entry.partner_name.trim().toLowerCase() !== "azul") {
    parts.push(entry.partner_name)
  }
  return parts
}

// A place is shown as its IATA code once assigned - exactly three upper-case letters. Anything
// else is a city name Azul sent that has no code yet, which the operator can assign.
function isAssignedCode(value: string): boolean {
  return /^[A-Z]{3}$/.test(value)
}

function formatPoints(points: number): string {
  const sign = points > 0 ? "+" : points < 0 ? "−" : ""
  return `${sign}${Math.abs(points).toLocaleString("pt-BR")}`
}

// "2026-09-10" -> "10/09/2026", without constructing a Date (which would shift by timezone).
function formatDay(day: string): string {
  const [year, month, dayOfMonth] = day.split("-")
  if (!year || !month || !dayOfMonth) return day
  return `${dayOfMonth}/${month}/${year}`
}

function routeOf(origin: string, destination: string): string | null {
  if (origin && destination) return `${origin} → ${destination}`
  return origin || destination || null
}

// How many movements one page shows. The window is loaded whole (filters and sort stay instant,
// client-side), but a long history would be an unreadable wall of rows, so it is paged here.
const STATEMENT_PAGE_SIZE = 100

interface AccountStatementViewProps {
  accountId: string
  accountName: string
  accountDocument: string | null
  program: LoyaltyProgramSummary
  // Which programme's routes to talk to, and which of the two airlines' quirks apply. It is
  // derived from the programme on the server rather than sniffed from the code here, so the
  // screen never guesses.
  syncedProgram: SyncedProgram
  entries: StatementEntries
}

export function AccountStatementView({
  accountId,
  accountName,
  accountDocument,
  program,
  syncedProgram,
  entries,
}: AccountStatementViewProps) {
  const [editMode, setEditMode] = useState(false)
  // Overrides keyed by locator, so linking one line updates every line that shares the locator
  // without a refetch. Undefined means "use the value the server sent".
  const [overrides, setOverrides] = useState<
    Record<string, StatementLinkedIssuance | null>
  >({})
  const [batchOverrides, setBatchOverrides] = useState<
    Record<string, StatementLinkedBatch | null>
  >({})
  // Client-side filters over the loaded window: the whole month is already here, so there is no
  // round-trip. Empty means no bound / all kinds.
  const [dateFrom, setDateFrom] = useState("")
  const [dateTo, setDateTo] = useState("")
  const [kindFilter, setKindFilter] = useState<StatementLineKind | "all">("all")
  // Newest first by default, which is how a statement is normally read; the operator can flip
  // it to oldest-first.
  const [sortOrder, setSortOrder] = useState<"desc" | "asc">("desc")
  // 1-based page over the filtered movements. Any filter or sort change resets it to the first page
  // (done in the handlers, not an effect), so the operator never lands on a page that no longer
  // exists.
  const [page, setPage] = useState(1)
  // Anchor at the top of the list, so paging brings the first row of the new page into view rather
  // than leaving the operator scrolled at the pager where they clicked.
  const listTopRef = useRef<HTMLSpanElement>(null)
  // First render must not scroll (the page loads at the top already); only a later page change does.
  const didMountRef = useRef(false)

  const { sync_state: syncState, from, to, lines } = entries

  function linkedFor(entry: StatementEntry): StatementLinkedIssuance | null {
    if (entry.key in overrides) {
      return overrides[entry.key]
    }
    return entry.linked_issuance ?? null
  }

  function linkedBatchFor(entry: StatementEntry): StatementLinkedBatch | null {
    if (entry.key in batchOverrides) {
      return batchOverrides[entry.key]
    }
    return entry.linked_batch ?? null
  }

  const syncMeta = SYNC_STATE_META[syncState.state]
  const balance =
    syncState.last_balance_points != null
      ? `${syncState.last_balance_points.toLocaleString("pt-BR")} pts`
      : "—"
  const lastSynced = syncState.last_synced_at
    ? formatDistanceToNow(new Date(syncState.last_synced_at), { addSuffix: true, locale: ptBR })
    : "nunca"

  // The lines that pass the filters. occurred_on is an ISO day, so string comparison is date
  // comparison. An empty bound is open on that side.
  const filteredLines = useMemo(() => {
    return lines.filter((line) => {
      if (dateFrom && line.occurred_on < dateFrom) return false
      if (dateTo && line.occurred_on > dateTo) return false
      if (kindFilter !== "all" && line.kind !== kindFilter) return false
      return true
    })
  }, [lines, dateFrom, dateTo, kindFilter])

  // The filtered lines in display order. The server sends them oldest-first, down to the order
  // within each day for Azul (its position in the day); the newest-first default reverses the
  // whole list, so a day also reads latest movement first, the way the airline's app shows it.
  const orderedLines = useMemo(
    () => (sortOrder === "desc" ? [...filteredLines].reverse() : filteredLines),
    [filteredLines, sortOrder],
  )

  const totalPages = Math.max(1, Math.ceil(orderedLines.length / STATEMENT_PAGE_SIZE))
  // Clamp rather than trust `page`: a filter that shrinks the list could otherwise leave it past
  // the end for a render before a handler resets it.
  const currentPage = Math.min(page, totalPages)
  const pageStart = (currentPage - 1) * STATEMENT_PAGE_SIZE
  const pageLines = orderedLines.slice(pageStart, pageStart + STATEMENT_PAGE_SIZE)

  // After a page change has rendered, jump the list top into view - otherwise a click on the pager
  // (which sits at the bottom) leaves the operator stranded in the middle of the new page. Runs
  // post-commit, so the new page's height is already settled and the scroll is not cut short.
  useEffect(() => {
    if (!didMountRef.current) {
      didMountRef.current = true
      return
    }
    listTopRef.current?.scrollIntoView({ block: "start" })
  }, [currentPage])

  // The current page's lines grouped by day for rendering. They are already in display order, so a
  // day split across two pages simply shows its header again on each - every line carries the same
  // day balance, so the partial day still reports it.
  const days = useMemo(() => {
    const groups: { day: string; balance: number | null; lines: StatementEntry[] }[] = []
    for (const line of pageLines) {
      const last = groups[groups.length - 1]
      if (last && last.day === line.occurred_on) {
        last.lines.push(line)
        if (last.balance == null && line.day_balance != null) last.balance = line.day_balance
      } else {
        groups.push({
          day: line.occurred_on,
          balance: line.day_balance ?? null,
          lines: [line],
        })
      }
    }
    return groups
  }, [pageLines])

  const hasFilters = dateFrom !== "" || dateTo !== "" || kindFilter !== "all"

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex flex-col gap-3 rounded-md border border-slate-200 p-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <LoyaltyProgramBadge loyaltyProgram={program} />
            <div className="flex flex-col">
              <h1 className="text-xl font-semibold text-slate-800">{accountName}</h1>
              {accountDocument && (
                <span className="text-xs text-slate-500">{accountDocument}</span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              asChild
              variant="outline"
              size="sm"
              title="Exportar CSV"
              className="h-7 gap-1 px-2 text-xs text-slate-600 hover:border-emerald-600 hover:bg-emerald-600 hover:text-white"
            >
              <a href={`/api/accounts/${accountId}/statement-export?program=${syncedProgram}`}>
                <Download className="h-3.5 w-3.5" />
                CSV
              </a>
            </Button>
            <Badge className={cn("font-medium", syncMeta.className)}>{syncMeta.label}</Badge>
          </div>
        </div>
        <div className="flex flex-wrap gap-x-8 gap-y-2 text-sm">
          <div className="flex flex-col">
            <span className="text-xs text-slate-500">Saldo</span>
            <span className="font-medium tabular-nums text-slate-800">{balance}</span>
          </div>
          <div className="flex flex-col">
            <span className="text-xs text-slate-500">Última sincronização</span>
            <span className="text-slate-700">{lastSynced}</span>
          </div>
          <div className="flex flex-col">
            <span className="text-xs text-slate-500">Período</span>
            <span className="text-slate-700">
              {from && to ? `${formatDay(from)} — ${formatDay(to)}` : "—"}
            </span>
          </div>
        </div>
        {syncState.detail && (
          <p className="text-xs text-slate-500">{syncState.detail}</p>
        )}
      </div>

      {/* Filters + edit-mode toggle */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <DateRangeFilter
            from={dateFrom}
            to={dateTo}
            onChange={({ from: nextFrom, to: nextTo }) => {
              setDateFrom(nextFrom)
              setDateTo(nextTo)
              setPage(1)
            }}
          />
          <Select
            value={kindFilter}
            onValueChange={(value) => {
              setKindFilter(value as StatementLineKind | "all")
              setPage(1)
            }}
          >
            <SelectTrigger className="w-44">
              <SelectValue placeholder="Tipo" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Todos os tipos</SelectItem>
              <SelectItem value="redemption">{KIND_META.redemption.label}</SelectItem>
              <SelectItem value="accrual">{KIND_META.accrual.label}</SelectItem>
              <SelectItem value="purchase">{KIND_META.purchase.label}</SelectItem>
              <SelectItem value="refund">{KIND_META.refund.label}</SelectItem>
              <SelectItem value="expiration">{KIND_META.expiration.label}</SelectItem>
              <SelectItem value="other">{KIND_META.other.label}</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={sortOrder}
            onValueChange={(value) => {
              setSortOrder(value as "desc" | "asc")
              setPage(1)
            }}
          >
            <SelectTrigger className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="desc">Mais recentes primeiro</SelectItem>
              <SelectItem value="asc">Mais antigas primeiro</SelectItem>
            </SelectContent>
          </Select>
          {hasFilters && (
            <Button
              variant="ghost"
              size="sm"
              className="text-slate-500"
              onClick={() => {
                setDateFrom("")
                setDateTo("")
                setKindFilter("all")
                setPage(1)
              }}
            >
              <X className="mr-1 h-3.5 w-3.5" />
              Limpar filtros
            </Button>
          )}
        </div>
        <label className="flex cursor-pointer items-center gap-2 text-sm text-slate-700">
          <Switch checked={editMode} onCheckedChange={setEditMode} />
          Editar vínculos
        </label>
      </div>

      <span ref={listTopRef} className="text-sm text-slate-500">
        {filteredLines.length}
        {hasFilters ? ` de ${lines.length}` : ""}{" "}
        {filteredLines.length === 1 ? "movimentação" : "movimentações"}
        {totalPages > 1 &&
          ` · mostrando ${pageStart + 1}–${Math.min(
            pageStart + STATEMENT_PAGE_SIZE,
            orderedLines.length,
          )}`}
      </span>

      {lines.length === 0 ? (
        <div className="rounded-md border border-dashed border-slate-200 p-8 text-center text-sm text-slate-500">
          Nenhuma movimentação sincronizada ainda. Conecte a conta e aguarde a primeira
          sincronização.
        </div>
      ) : filteredLines.length === 0 ? (
        <div className="rounded-md border border-dashed border-slate-200 p-8 text-center text-sm text-slate-500">
          Nenhuma movimentação no filtro escolhido.
        </div>
      ) : (
        <div className="flex flex-col gap-5">
          {days.map((group) => (
            <div key={group.day} className="flex flex-col gap-2">
              <div className="flex items-baseline justify-between gap-2">
                <div className="flex items-baseline gap-2">
                  <h2 className="text-xs font-semibold tracking-wide text-slate-400 uppercase">
                    {formatDay(group.day)}
                  </h2>
                </div>
                {group.balance != null && (
                  <span className="text-xs text-slate-400 tabular-nums">
                    saldo do dia {group.balance.toLocaleString("pt-BR")}
                  </span>
                )}
              </div>
              <div className="flex flex-col divide-y divide-slate-100 rounded-md border border-slate-200">
                {group.lines.map((entry, index) => (
                  <StatementLine
                    key={`${group.day}-${index}`}
                    accountId={accountId}
                    syncedProgram={syncedProgram}
                    entry={entry}
                    linked={linkedFor(entry)}
                    linkedBatch={linkedBatchFor(entry)}
                    editMode={editMode}
                    onLinkedChange={(lineKey, linked) =>
                      setOverrides((prev) => ({ ...prev, [lineKey]: linked }))
                    }
                    onBatchLinkedChange={(lineKey, linked) =>
                      setBatchOverrides((prev) => ({ ...prev, [lineKey]: linked }))
                    }
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-2 pt-1">
          <span className="text-xs text-slate-500">
            Página {currentPage} de {totalPages}
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="sm"
              className="h-8"
              disabled={currentPage <= 1}
              onClick={() => setPage(currentPage - 1)}
            >
              <ChevronLeft className="mr-1 h-3.5 w-3.5" />
              Anterior
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="h-8"
              disabled={currentPage >= totalPages}
              onClick={() => setPage(currentPage + 1)}
            >
              Próxima
              <ChevronRight className="ml-1 h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

interface StatementLineProps {
  accountId: string
  syncedProgram: SyncedProgram
  entry: StatementEntry
  linked: StatementLinkedIssuance | null
  linkedBatch: StatementLinkedBatch | null
  editMode: boolean
  onLinkedChange: (linkKey: string, linked: StatementLinkedIssuance | null) => void
  onBatchLinkedChange: (linkKey: string, linked: StatementLinkedBatch | null) => void
}

function StatementLine({
  accountId,
  syncedProgram,
  entry,
  linked,
  linkedBatch,
  editMode,
  onLinkedChange,
  onBatchLinkedChange,
}: StatementLineProps) {
  const meta = KIND_META[entry.kind]
  const Icon = meta.icon
  const detail = detailParts(entry).join(" · ")
  const linkable = isLinkable(entry.kind)
  const batchLinkable = isBatchLinkable(entry.kind)

  return (
    <div className="flex flex-col gap-2 px-4 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-2">
          <Icon className={cn("mt-0.5 h-4 w-4 shrink-0", meta.className)} />
          <div className="flex min-w-0 flex-col">
            <span className="text-sm font-medium text-slate-800">
              {meta.label}
              {entry.locator && (
                <span className="ml-2 font-mono text-xs text-slate-500">{entry.locator}</span>
              )}
            </span>
            {(entry.origin || entry.destination) && (
              <span className="flex items-center gap-1 text-xs text-slate-500">
                {/* Azul sends a city name, which has to be mapped to an IATA code by hand;
                    Smiles sends the code itself, so there is nothing to assign. */}
                {syncedProgram === "azul" ? (
                  <>
                    <CityChip accountId={accountId} value={entry.origin} editMode={editMode} />
                    {entry.origin && entry.destination && <span>→</span>}
                    <CityChip accountId={accountId} value={entry.destination} editMode={editMode} />
                  </>
                ) : (
                  <span className="font-mono">{routeOf(entry.origin, entry.destination)}</span>
                )}
              </span>
            )}
            {detail && (
              <span className="text-xs break-words text-slate-500">{detail}</span>
            )}
          </div>
        </div>
        <div className="flex flex-col items-end">
          <span className={cn("text-sm font-semibold tabular-nums", meta.className)}>
            {formatPoints(entry.points)}
          </span>
        </div>
      </div>

      {linkable && (
        <BookingLink
          accountId={accountId}
          syncedProgram={syncedProgram}
          linkKey={entry.key}
          linked={linked}
          editMode={editMode}
          onLinkedChange={onLinkedChange}
        />
      )}

      {batchLinkable && (
        <BatchLink
          accountId={accountId}
          syncedProgram={syncedProgram}
          linkKey={entry.key}
          occurredOn={entry.occurred_on}
          points={entry.points}
          linked={linkedBatch}
          editMode={editMode}
          onLinkedChange={onBatchLinkedChange}
        />
      )}
    </div>
  )
}

interface BookingLinkProps {
  accountId: string
  syncedProgram: SyncedProgram
  linkKey: string
  linked: StatementLinkedIssuance | null
  editMode: boolean
  onLinkedChange: (linkKey: string, linked: StatementLinkedIssuance | null) => void
}

function BookingLink({
  accountId,
  syncedProgram,
  linkKey,
  linked,
  editMode,
  onLinkedChange,
}: BookingLinkProps) {
  const [isPending, startTransition] = useTransition()

  function handleUnlink() {
    startTransition(async () => {
      const result = await unlinkStatementBookingAction(syncedProgram, accountId, linkKey)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      onLinkedChange(linkKey, result.linked)
      toast.success("Vínculo removido.")
    })
  }

  function handlePick(issuance: IssuanceLinkOption) {
    startTransition(async () => {
      const result = await linkStatementBookingAction(syncedProgram, accountId, linkKey, issuance.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      onLinkedChange(linkKey, result.linked)
      toast.success("Vínculo salvo.")
    })
  }

  return (
    <div className="ml-6 flex flex-wrap items-center gap-2 rounded-md bg-slate-50 px-3 py-2 text-xs">
      {linked ? (
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <Link2 className="h-3.5 w-3.5 shrink-0 text-slate-500" />
          <span className="min-w-0 truncate text-slate-700">
            {linked.passenger_name || "Passageiro não informado"}
            {routeOf(linked.origin, linked.destination) && (
              <span className="text-slate-500">
                {" "}
                · {routeOf(linked.origin, linked.destination)}
              </span>
            )}
            <span className="text-slate-400"> · {formatDay(linked.issued_on)}</span>
          </span>
          {linked.manual && (
            <Badge className="bg-cyan-100 text-cyan-700">manual</Badge>
          )}
        </div>
      ) : (
        <div className="flex flex-1 items-center gap-2 text-slate-500">
          <Link2Off className="h-3.5 w-3.5 shrink-0" />
          Não vinculada
        </div>
      )}

      {editMode && (
        <div className="flex items-center gap-1">
          {isPending && <Loader2 className="h-3.5 w-3.5 animate-spin text-slate-400" />}
          <IssuancePicker accountId={accountId} disabled={isPending} onPick={handlePick} />
          {/* Only a manual pin can be cleared: an automatic match has nothing to remove, and
              "Limpar" there would be a no-op that reads as broken. */}
          {linked?.manual && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-slate-500 hover:text-rose-600"
              disabled={isPending}
              onClick={handleUnlink}
            >
              <Link2Off className="mr-1 h-3.5 w-3.5" />
              Limpar
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

interface IssuancePickerProps {
  accountId: string
  disabled: boolean
  onPick: (issuance: IssuanceLinkOption) => void
}

// A small search box in a popover: type a locator, pick the issuance. It reuses the account's
// issuance search, so only bookings on this account show up.
function IssuancePicker({ accountId, disabled, onPick }: IssuancePickerProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [results, setResults] = useState<IssuanceLinkOption[]>([])
  const [isSearching, startSearching] = useTransition()

  function runSearch(value: string) {
    setQuery(value)
    startSearching(async () => {
      const result = await searchAccountIssuancesAction(accountId, value)
      if ("error" in result) {
        setResults([])
        return
      }
      setResults(result.issuances)
    })
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          setQuery("")
          setResults([])
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 px-2 text-cyan-700 hover:text-cyan-800"
          disabled={disabled}
        >
          <Link2 className="mr-1 h-3.5 w-3.5" />
          Vincular
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-2">
        <div className="flex items-center gap-2 rounded-md border border-slate-200 px-2">
          <Search className="h-3.5 w-3.5 text-slate-400" />
          <input
            autoFocus
            value={query}
            onChange={(event) => runSearch(event.target.value)}
            placeholder="Buscar por localizador…"
            className="w-full bg-transparent py-1.5 text-sm outline-none"
          />
          {query && (
            <button
              type="button"
              onClick={() => runSearch("")}
              className="text-slate-400 hover:text-slate-600"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
        <div className="mt-2 max-h-64 overflow-y-auto">
          {isSearching ? (
            <div className="flex items-center justify-center gap-2 py-4 text-xs text-slate-400">
              <RefreshCcw className="h-3.5 w-3.5 animate-spin" />
              Buscando…
            </div>
          ) : query.trim() === "" ? (
            <p className="py-4 text-center text-xs text-slate-400">
              Digite um localizador para buscar.
            </p>
          ) : results.length === 0 ? (
            <p className="py-4 text-center text-xs text-slate-400">Nenhuma emissão encontrada.</p>
          ) : (
            <ul className="flex flex-col">
              {results.map((issuance) => (
                <li key={issuance.id}>
                  <button
                    type="button"
                    onClick={() => {
                      onPick(issuance)
                      setOpen(false)
                    }}
                    className="flex w-full flex-col items-start rounded-md px-2 py-1.5 text-left hover:bg-slate-100"
                  >
                    <span className="font-mono text-xs text-slate-700">
                      {issuance.program_locator}
                    </span>
                    <span className="text-xs text-slate-500">
                      {[
                        issuance.main_passenger_name,
                        routeOf(
                          issuance.origin_iata_code ?? "",
                          issuance.destination_iata_code ?? "",
                        ),
                        formatDay(issuance.issued_on),
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

interface BatchLinkProps {
  accountId: string
  syncedProgram: SyncedProgram
  linkKey: string
  occurredOn: string
  points: number
  linked: StatementLinkedBatch | null
  editMode: boolean
  onLinkedChange: (linkKey: string, linked: StatementLinkedBatch | null) => void
}

// The lote side of a line, mirroring BookingLink: shows the batch a purchase/accrual maps to
// (origin, cost, acquisition date) or "não vinculado", and in edit mode lets the operator pick or
// clear a batch. The day and points ride along so the server can recompute the auto match.
function BatchLink({
  accountId,
  syncedProgram,
  linkKey,
  occurredOn,
  points,
  linked,
  editMode,
  onLinkedChange,
}: BatchLinkProps) {
  const [isPending, startTransition] = useTransition()

  function handleUnlink() {
    startTransition(async () => {
      const result = await unlinkStatementBatchAction(syncedProgram, accountId, linkKey, occurredOn, points)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      onLinkedChange(linkKey, result.linked)
      toast.success("Vínculo removido.")
    })
  }

  function handlePick(batch: BatchLinkOption) {
    startTransition(async () => {
      const result = await linkStatementBatchAction(syncedProgram, accountId, linkKey, batch.id, occurredOn, points)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      onLinkedChange(linkKey, result.linked)
      toast.success("Vínculo salvo.")
    })
  }

  return (
    <div className="ml-6 flex flex-wrap items-center gap-2 rounded-md bg-slate-50 px-3 py-2 text-xs">
      {linked ? (
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <Boxes className="h-3.5 w-3.5 shrink-0 text-slate-500" />
          <span className="min-w-0 truncate text-slate-700">
            {linked.origin}
            <span className="text-slate-500"> · {formatCurrencyBRL(linked.cost_cents)}</span>
            <span className="text-slate-400"> · {formatDay(linked.acquired_on)}</span>
          </span>
          {linked.manual && <Badge className="bg-cyan-100 text-cyan-700">manual</Badge>}
        </div>
      ) : (
        <div className="flex flex-1 items-center gap-2 text-slate-500">
          <Boxes className="h-3.5 w-3.5 shrink-0" />
          Sem lote vinculado
        </div>
      )}

      {editMode && (
        <div className="flex items-center gap-1">
          {isPending && <Loader2 className="h-3.5 w-3.5 animate-spin text-slate-400" />}
          <BatchPicker accountId={accountId} disabled={isPending} onPick={handlePick} />
          {linked?.manual && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-slate-500 hover:text-rose-600"
              disabled={isPending}
              onClick={handleUnlink}
            >
              <Link2Off className="mr-1 h-3.5 w-3.5" />
              Limpar
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

interface BatchPickerProps {
  accountId: string
  disabled: boolean
  onPick: (batch: BatchLinkOption) => void
}

// A popover listing the account's batches to pin a point-entry line to. There are few per
// account, so it loads them once on open and filters by origin as the operator types.
function BatchPicker({ accountId, disabled, onPick }: BatchPickerProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [batches, setBatches] = useState<BatchLinkOption[]>([])
  const [isLoading, startLoading] = useTransition()

  function load() {
    startLoading(async () => {
      const result = await getAccountBatchesAction(accountId)
      setBatches("error" in result ? [] : result.batches)
    })
  }

  const filtered =
    query.trim() === ""
      ? batches
      : batches.filter((b) => b.origin.toLowerCase().includes(query.trim().toLowerCase()))

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) {
          setQuery("")
          load()
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 px-2 text-cyan-700 hover:text-cyan-800"
          disabled={disabled}
        >
          <Package className="mr-1 h-3.5 w-3.5" />
          Vincular lote
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-2">
        <div className="flex items-center gap-2 rounded-md border border-slate-200 px-2">
          <Search className="h-3.5 w-3.5 text-slate-400" />
          <input
            autoFocus
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Filtrar por origem…"
            className="w-full bg-transparent py-1.5 text-sm outline-none"
          />
        </div>
        <div className="mt-2 max-h-64 overflow-y-auto">
          {isLoading ? (
            <div className="flex items-center justify-center gap-2 py-4 text-xs text-slate-400">
              <RefreshCcw className="h-3.5 w-3.5 animate-spin" />
              Carregando…
            </div>
          ) : filtered.length === 0 ? (
            <p className="py-4 text-center text-xs text-slate-400">Nenhum lote encontrado.</p>
          ) : (
            <ul className="flex flex-col">
              {filtered.map((batch) => (
                <li key={batch.id}>
                  <button
                    type="button"
                    onClick={() => {
                      onPick(batch)
                      setOpen(false)
                    }}
                    className="flex w-full flex-col items-start rounded-md px-2 py-1.5 text-left hover:bg-slate-100"
                  >
                    <span className="text-xs font-medium text-slate-700">{batch.origin}</span>
                    <span className="text-xs text-slate-500">
                      {batch.points.toLocaleString("pt-BR")} pts ·{" "}
                      {formatCurrencyBRL(batch.cost_cents)} · {formatDay(batch.acquired_on)}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

interface CityChipProps {
  accountId: string
  value: string
  editMode: boolean
}

// One endpoint of a redemption's route. Shown as its IATA code once assigned; a city Azul sent
// with no code yet is highlighted and, in edit mode, becomes a small assigner where the operator
// types the three-letter code. Saving applies to every line with that city (the mapping is
// global), so the page refreshes to pick it up.
function CityChip({ accountId, value, editMode }: CityChipProps) {
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [code, setCode] = useState("")
  const [isPending, startTransition] = useTransition()

  if (!value) return null
  if (isAssignedCode(value)) {
    return <span className="font-medium text-slate-600">{value}</span>
  }

  // An unassigned city.
  if (!editMode) {
    return (
      <span className="rounded bg-amber-100 px-1 text-amber-800" title="Cidade sem código IATA">
        {value}
      </span>
    )
  }

  function handleSave() {
    startTransition(async () => {
      const result = await assignAzulCityCodeAction(accountId, value, code)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      setOpen(false)
      toast.success(`${value} → ${code.toUpperCase()}`)
      router.refresh()
    })
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) setCode("")
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          className="rounded bg-amber-100 px-1 text-amber-800 underline decoration-dotted hover:bg-amber-200"
          title="Atribuir código IATA"
        >
          {value}
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 p-3">
        <p className="mb-2 text-xs text-slate-600">
          Código IATA de <span className="font-medium">{value}</span>
        </p>
        <div className="flex items-center gap-2">
          <input
            autoFocus
            value={code}
            onChange={(event) =>
              setCode(event.target.value.replace(/[^A-Za-z]/g, "").slice(0, 3).toUpperCase())
            }
            onKeyDown={(event) => {
              if (event.key === "Enter" && code.length === 3) handleSave()
            }}
            placeholder="XYZ"
            className="w-20 rounded-md border border-slate-200 px-2 py-1 text-sm uppercase outline-none focus:border-cyan-500"
          />
          <Button
            size="sm"
            className="h-8"
            disabled={isPending || code.length !== 3}
            onClick={handleSave}
          >
            {isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : "Salvar"}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
