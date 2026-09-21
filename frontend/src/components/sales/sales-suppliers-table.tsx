"use client"

import { useMemo, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import {
  Check,
  CheckCheck,
  ChevronDown,
  ChevronRight,
  EyeOff,
  FileText,
  Minus,
  Square,
  TriangleAlert,
  X,
} from "lucide-react"
import { toast } from "sonner"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { SupplierCostCell } from "@/components/sales/supplier-cost-cell"
import {
  sumSupplierBalances,
  sumSupplierLines,
  supplierLineAmount,
} from "@/components/sales/supplier-amount"
import { updateSupplierIssuancesAction } from "@/actions/supplier-issuances"
import {
  incrementSupplierExportCounterAction,
  setSupplierExportCounterAction,
} from "@/actions/supplier-export-counter"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { formatPoints } from "@/lib/format"
import { normalizeSearch } from "@/lib/search"
import {
  GROUP_BY_ADMINISTRATOR,
  NO_ADMINISTRATOR_KEY,
} from "@/app/dashboard/sales/period-params"
import { cn } from "@/lib/utils"
import type { SupplierBalance, SupplierIssuance } from "@/schemas/sale"

interface SalesSuppliersTableProps {
  balances: SupplierBalance[]
  // "account" or "administrator". Resolved by the page from the URL so the table, the
  // document and the address bar can never disagree about what a row is.
  groupBy: string
  // The query the page used to fetch these balances, so the PDF covers exactly the same
  // set. Passed as a string rather than rebuilt here: rebuilding is how the document and
  // the screen start disagreeing.
  query: string
}

// The account row and the issuance rows share these columns and mean the same thing in
// both, which is the whole point of the layout: an account's own figures sit under the
// same headings its issuances do.
//
//   1 chevron
//   2 Conta            / Localizador
//   3 Programa         / Data
//   4 Administrador    / Trecho
//   5 Milhas líquidas  / Pontos       (a line's signed points are its net contribution)
//   6 Taxas
//   7 Milheiro de custo
//   8 Valor
//   9 Conferido
const COLUMN_COUNT = 9

type PaymentTerms = {
  costOfTheThousand?: number | null
  taxesIncludedInCost?: boolean
  reviewed?: boolean
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("pt-BR")
}

// `hidden` only reaches the document on the report that covers everything. The per-account
// one is already narrowed to a single account, and excluding others from it would either be
// a no-op or, if that account were itself hidden, produce an empty PDF from a button the
// operator just pressed.
function reportHref(
  query: string,
  excludedAccountIds: string[],
  group?: { key: string; byAdministrator: boolean },
): string {
  const params = new URLSearchParams(query)
  if (group) {
    params.set(group.byAdministrator ? "administrator_id" : "account_id", group.key)
  } else if (excludedAccountIds.length > 0) {
    params.set("exclude_account_ids", excludedAccountIds.join(","))
  }
  return `/api/sales/suppliers/report?${params.toString()}`
}

// The PDF button refuses before the request rather than after: the backend answers 422 for
// a missing cost anyway, but finding out after a download attempt is a worse way to learn
// that four issuances still need a rate.
function ReportButton({
  href,
  missingCost,
  label,
  iconOnly,
}: {
  href: string
  missingCost: number
  label: string
  iconOnly?: boolean
}) {
  const blocked = missingCost > 0
  const button = (
    <Button
      asChild={!blocked}
      type="button"
      size={iconOnly ? "icon" : "sm"}
      variant="outline"
      disabled={blocked}
      className={iconOnly ? "h-7 w-7" : undefined}
      aria-label={label}
    >
      {blocked ? (
        <span>
          <FileText className="h-4 w-4" />
          {!iconOnly && <span className="ml-1.5">{label}</span>}
        </span>
      ) : (
        <a href={href} download>
          <FileText className="h-4 w-4" />
          {!iconOnly && <span className="ml-1.5">{label}</span>}
        </a>
      )}
    </Button>
  )

  if (!blocked) return button

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex cursor-not-allowed">{button}</span>
      </TooltipTrigger>
      <TooltipContent>
        {missingCost === 1
          ? "1 emissão está sem milheiro de custo."
          : `${missingCost} emissões estão sem milheiro de custo.`}
      </TooltipContent>
    </Tooltip>
  )
}

// The PDF file name: "resumo-<administrador>-<contador>.pdf". Built here rather than left to the
// backend's Content-Disposition because the route is same-origin, so the <a download> value wins -
// which is what lets the per-export counter reach the file name without a round trip through the
// GET that streams the PDF.
function reportFileName(label: string, counter: number): string {
  const slug =
    normalizeSearch(label)
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "fornecedor"
  return `resumo-${slug}-${String(counter).padStart(4, "0")}.pdf`
}

// The per-administrator export: an editable counter and the PDF button beside it. Pressing the
// button bumps the counter (server-side, atomically) and downloads the report named with the new
// number; the input lets the operator correct the count or start a fresh sequence by hand.
function AdministratorExport({
  administratorId,
  label,
  href,
  missingCost,
  initialCounter,
}: {
  administratorId: string
  label: string
  href: string
  missingCost: number
  initialCounter: number
}) {
  const [counter, setCounter] = useState(initialCounter)
  const [text, setText] = useState(String(initialCounter))
  const [exporting, setExporting] = useState(false)
  const [, startTransition] = useTransition()
  const blocked = missingCost > 0

  async function handleExport() {
    if (blocked || exporting) return
    setExporting(true)
    const result = await incrementSupplierExportCounterAction(administratorId)
    setExporting(false)
    if ("error" in result) {
      toast.error(result.error)
      return
    }
    setCounter(result.counter)
    setText(String(result.counter))

    // Programmatic download so the file name carries the fresh number: the same-origin route
    // lets the download attribute override the backend's Content-Disposition.
    const anchor = document.createElement("a")
    anchor.href = href
    anchor.download = reportFileName(label, result.counter)
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  }

  function commitCounter() {
    const value = Number(text)
    if (!Number.isInteger(value) || value < 0) {
      setText(String(counter))
      return
    }
    if (value === counter) return
    startTransition(async () => {
      const result = await setSupplierExportCounterAction(administratorId, value)
      if ("error" in result) {
        toast.error(result.error)
        setText(String(counter))
        return
      }
      setCounter(result.counter)
      setText(String(result.counter))
    })
  }

  const button = (
    <Button
      type="button"
      size="icon"
      variant="outline"
      className="h-7 w-7"
      disabled={blocked || exporting}
      aria-label={`Gerar PDF de ${label}`}
      onClick={handleExport}
    >
      <FileText className="h-4 w-4" />
    </Button>
  )

  return (
    <span className="flex items-center gap-1">
      <Input
        value={text}
        onChange={(e) => setText(e.target.value)}
        onBlur={commitCounter}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault()
            ;(e.target as HTMLInputElement).blur()
          }
        }}
        inputMode="numeric"
        aria-label={`Contador de exportação de ${label}`}
        title="Número que vai no nome do PDF; sobe a cada exportação"
        className="h-7 w-12 text-center text-xs"
      />
      {blocked ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex cursor-not-allowed">{button}</span>
          </TooltipTrigger>
          <TooltipContent>
            {missingCost === 1
              ? "1 emissão está sem milheiro de custo."
              : `${missingCost} emissões estão sem milheiro de custo.`}
          </TooltipContent>
        </Tooltip>
      ) : (
        button
      )}
    </span>
  )
}

// A row is an account or an administrator depending on the grouping, and the fields that
// identify it are new. Falling back to the account ones keeps the screen working against an
// API published before this - which is a normal state here, not an edge case.
function groupKeyOf(balance: SupplierBalance): string {
  return balance.group_key ?? balance.account_id
}

function groupLabelOf(balance: SupplierBalance): string {
  return balance.group_label ?? balance.account_label
}

export function SalesSuppliersTable({ balances, query, groupBy }: SalesSuppliersTableProps) {
  const byAdministrator = groupBy === GROUP_BY_ADMINISTRATOR
  const router = useRouter()
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [saving, setSaving] = useState<Set<string>>(new Set())
  const [bulkText, setBulkText] = useState("")
  const [pendingBulk, setPendingBulk] = useState<{ terms: PaymentTerms; summary: string } | null>(
    null,
  )
  const [, startTransition] = useTransition()

  // Accounts the operator has parked while reconciling. Deliberately component state and
  // not a URL key or a sticky filter: it dies with the screen, so a reload always shows the
  // whole period again. An account still hidden tomorrow is how a month gets closed against
  // a total that quietly left something out.
  const [hidden, setHidden] = useState<Set<string>>(new Set())

  const visible = useMemo(
    () => balances.filter((balance) => !hidden.has(groupKeyOf(balance))),
    [balances, hidden],
  )

  // The document is filtered by account whichever way the screen is grouped, because that
  // is the column the backend can exclude on. Hiding an administrator therefore means
  // hiding every account behind them, which is what the row said it would do.
  const excludedAccountIds = useMemo(
    () =>
      [
        ...new Set(
          balances
            .filter((balance) => hidden.has(groupKeyOf(balance)))
            .flatMap((balance) => balance.issuances.map((line) => line.account_id ?? balance.account_id)),
        ),
      ].filter(Boolean) as string[],
    [balances, hidden],
  )
  const hiddenCount = balances.length - visible.length

  // Summed in the browser rather than asked of the API: the table does not paginate, so
  // these are exactly the rows on screen - which is what "total do que está aparecendo"
  // means. A server-side total could disagree with the visible rows and nobody would know
  // which one to trust. Hiding an account is the same argument, one step further: it leaves
  // the total because it left the screen.
  const totals = useMemo(() => sumSupplierBalances(visible), [visible])

  // Over the visible accounts, so the bulk buttons write to what the operator is looking at.
  // Taking the hidden ones out of the total but leaving them in here would mark as settled
  // an account they had just decided not to look at.
  const allIssuanceIds = useMemo(
    () => [...new Set(visible.flatMap((b) => b.issuances.map((i) => i.issuance_id)))],
    [visible],
  )

  // Counted over the visible lines, for the same reason the money totals are: the footer
  // must describe the rows on screen and nothing else.
  const reviewedSummary = useMemo(() => {
    const lines = visible.flatMap((balance) => balance.issuances)
    return { reviewed: lines.filter((line) => line.reviewed).length, total: lines.length }
  }, [visible])

  function hideGroup(key: string) {
    setHidden((current) => new Set(current).add(key))
  }

  function toggleExpanded(accountID: string) {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(accountID)) next.delete(accountID)
      else next.add(accountID)
      return next
    })
  }

  function commit(issuanceIds: string[], terms: PaymentTerms, describe: (n: number) => string) {
    setSaving((current) => new Set([...current, ...issuanceIds]))

    startTransition(async () => {
      const result = await updateSupplierIssuancesAction(issuanceIds, terms)
      setSaving((current) => {
        const next = new Set(current)
        for (const id of issuanceIds) next.delete(id)
        return next
      })

      if ("error" in result && result.error) {
        toast.error(result.error)
        return
      }
      toast.success(describe(result.updated ?? issuanceIds.length))
      // The action revalidates the page; this is what makes the server's answer - not an
      // optimistic guess - the value the cells settle on.
      router.refresh()
    })
  }

  function askBulk(terms: PaymentTerms, summary: string) {
    if (allIssuanceIds.length === 0) return
    setPendingBulk({ terms, summary })
  }

  function confirmBulk() {
    if (!pendingBulk) return
    commit(allIssuanceIds, pendingBulk.terms, (n) =>
      n === 1 ? "1 emissão atualizada." : `${n} emissões atualizadas.`,
    )
    setPendingBulk(null)
    setBulkText("")
  }

  const bulkCents = parseCurrencyBRLInput(bulkText)

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-end gap-3">
        {/* Hiding is invisible by nature, and a total that silently leaves rows out is the
            one thing this screen must never do. The count is the receipt. */}
        {hiddenCount > 0 && (
          <span className="flex items-center gap-2 text-sm text-muted-foreground">
            <EyeOff className="h-4 w-4" />
            {hiddenCount === 1 ? "1 conta oculta" : `${hiddenCount} contas ocultas`}
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-7 px-2 text-xs"
              onClick={() => setHidden(new Set())}
            >
              Mostrar todas
            </Button>
          </span>
        )}
        <ReportButton
          href={reportHref(query, excludedAccountIds)}
          missingCost={totals.missingCost}
          label="Gerar PDF de todas"
        />
      </div>

      <div className="max-h-[calc(100vh-24rem)] overflow-auto rounded-md border">
        <Table>
          <TableHeader className="sticky top-0 z-10 bg-slate-100">
            <TableRow>
              <TableHead className="w-8" />
              {/* The first column is whatever a row is, and the fourth carries what the
                  grouping took away: grouped by administrator the programme is no longer a
                  single value, so the useful thing to show is how many accounts fed it. */}
              <TableHead>{byAdministrator ? "Administrador" : "Conta"}</TableHead>
              <TableHead>Programa</TableHead>
              <TableHead>{byAdministrator ? "Contas" : "Administrador"}</TableHead>
              <TableHead className="text-right">Milhas líquidas</TableHead>

              {/* The two bulk controls live in the header of the column they write to,
                  so what they affect is never in doubt. */}
              <TableHead className="text-right">
                <div className="flex flex-col items-end gap-1 py-1">
                  <span>Taxas</span>
                  <div className="flex gap-1">
                    <Button
                      type="button"
                      size="icon"
                      variant="outline"
                      className="h-6 w-6"
                      disabled={allIssuanceIds.length === 0}
                      aria-label="Incluir as taxas em todas as emissões filtradas"
                      onClick={() =>
                        askBulk({ taxesIncludedInCost: true }, "Incluir as taxas")
                      }
                    >
                      <Check className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      type="button"
                      size="icon"
                      variant="outline"
                      className="h-6 w-6"
                      disabled={allIssuanceIds.length === 0}
                      aria-label="Tirar as taxas de todas as emissões filtradas"
                      onClick={() => askBulk({ taxesIncludedInCost: false }, "Tirar as taxas")}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              </TableHead>

              <TableHead className="text-right">
                <div className="flex flex-col items-end gap-1 py-1">
                  <span>Milheiro de custo</span>
                  <div className="flex gap-1">
                    <Input
                      value={bulkText}
                      onChange={(e) => setBulkText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && bulkCents !== null) {
                          e.preventDefault()
                          askBulk(
                            { costOfTheThousand: bulkCents },
                            `Aplicar ${formatCurrencyBRL(bulkCents)}`,
                          )
                        }
                      }}
                      placeholder="R$ 16,50"
                      inputMode="decimal"
                      aria-label="Milheiro de custo para todas as emissões filtradas"
                      className="h-6 w-24 text-right text-xs"
                    />
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      className="h-6 px-2 text-xs"
                      disabled={bulkCents === null || allIssuanceIds.length === 0}
                      onClick={() =>
                        bulkCents !== null &&
                        askBulk(
                          { costOfTheThousand: bulkCents },
                          `Aplicar ${formatCurrencyBRL(bulkCents)}`,
                        )
                      }
                    >
                      Todas
                    </Button>
                  </div>
                </div>
              </TableHead>

              <TableHead className="text-right">Valor</TableHead>

              {/* Same placement rule as the two controls above: the bulk button lives in
                  the header of the column it writes to. It covers the visible accounts,
                  so a hidden one is not quietly signed off. */}
              <TableHead className="text-center">
                <div className="flex flex-col items-center gap-1 py-1">
                  <span>Conferido</span>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="h-6 px-2 text-xs"
                    disabled={allIssuanceIds.length === 0}
                    onClick={() => askBulk({ reviewed: true }, "Marcar como conferidas")}
                  >
                    Todas
                  </Button>
                </div>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={COLUMN_COUNT}
                  className="py-10 text-center text-sm text-muted-foreground"
                >
                  {balances.length === 0
                    ? "Nenhuma emissão no período selecionado."
                    : "Tudo que o período tinha está oculto."}
                </TableCell>
              </TableRow>
            )}
            {visible.map((balance) => (
              <SupplierAccountRows
                key={groupKeyOf(balance)}
                balance={balance}
                byAdministrator={byAdministrator}
                isOpen={expanded.has(groupKeyOf(balance))}
                onToggle={() => toggleExpanded(groupKeyOf(balance))}
                onHide={() => hideGroup(groupKeyOf(balance))}
                lines={sumSupplierLines(balance.issuances)}
                saving={saving}
                reportHref={reportHref(query, excludedAccountIds, {
                  key: groupKeyOf(balance),
                  byAdministrator,
                })}
                onCommit={commit}
              />
            ))}
          </TableBody>
          {visible.length > 0 && (
            <TableFooter className="sticky bottom-0 bg-slate-100">
              <TableRow>
                <TableCell colSpan={4} className="font-medium">
                  Total ({visible.length}{" "}
                  {byAdministrator
                    ? visible.length === 1
                      ? "administrador"
                      : "administradores"
                    : visible.length === 1
                      ? "conta"
                      : "contas"}
                  )
                </TableCell>
                <TableCell
                  className={cn(
                    "text-right font-semibold tabular-nums",
                    totals.net < 0 && "text-red-700",
                  )}
                >
                  {formatPoints(totals.net)}
                </TableCell>
                <TableCell className="text-right font-medium tabular-nums text-muted-foreground">
                  {formatCurrencyBRL(totals.taxes)}
                </TableCell>
                <TableCell />
                <TableCell
                  className={cn(
                    "text-right font-semibold tabular-nums",
                    totals.amount < 0 && "text-red-700",
                  )}
                >
                  {formatCurrencyBRL(totals.amount)}
                </TableCell>
                <TableCell className="text-center text-xs font-medium tabular-nums text-muted-foreground">
                  {reviewedSummary.reviewed}/{reviewedSummary.total}
                </TableCell>
              </TableRow>
            </TableFooter>
          )}
        </Table>
      </div>

      {totals.missingCost > 0 && (
        <p className="flex items-center gap-1.5 text-sm text-amber-700">
          <TriangleAlert className="h-4 w-4 shrink-0" />
          {totals.missingCost === 1
            ? "1 emissão ainda está sem milheiro de custo e não entra no valor."
            : `${totals.missingCost} emissões ainda estão sem milheiro de custo e não entram no valor.`}
        </p>
      )}

      <AlertDialog
        open={pendingBulk !== null}
        onOpenChange={(open) => !open && setPendingBulk(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {pendingBulk?.summary} em {allIssuanceIds.length}{" "}
              {allIssuanceIds.length === 1 ? "emissão" : "emissões"}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Isto grava em todas as emissões filtradas no momento, substituindo o que já
              estava definido nelas. Não há como desfazer em lote.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction onClick={confirmBulk}>Aplicar</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function SupplierAccountRows({
  balance,
  byAdministrator,
  isOpen,
  onToggle,
  onHide,
  lines,
  saving,
  reportHref,
  onCommit,
}: {
  balance: SupplierBalance
  byAdministrator: boolean
  isOpen: boolean
  onToggle: () => void
  onHide: () => void
  lines: ReturnType<typeof sumSupplierLines>
  saving: Set<string>
  reportHref: string
  onCommit: (ids: string[], terms: PaymentTerms, describe: (n: number) => string) => void
}) {
  // An issuance created and cancelled in the same period is two lines from one row, so
  // both carry the same flag - counting lines rather than issuances is still right here,
  // because they agree by construction.
  const reviewedCount = balance.issuances.filter((line) => line.reviewed).length
  const allReviewed = reviewedCount > 0 && reviewedCount === balance.issuances.length
  const someReviewed = reviewedCount > 0 && !allReviewed

  return (
    <>
      <TableRow className="cursor-pointer" onClick={onToggle}>
        <TableCell className="w-8 text-muted-foreground">
          {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </TableCell>
        <TableCell className="font-medium">
          <span className="flex items-center gap-2">
            {groupLabelOf(balance)}
            <span onClick={(e) => e.stopPropagation()} className="flex items-center gap-1">
              {/* The bucket for accounts nobody administers is not an administrator id, so
                  there is no filter that would reproduce this row in a document. Better no
                  button than one that generates the wrong PDF. */}
              {groupKeyOf(balance) !== NO_ADMINISTRATOR_KEY &&
                (byAdministrator ? (
                  // Per administrator: the export is numbered, so it goes through the counter
                  // flow (increment, then download named with the new number) instead of a
                  // plain link.
                  <AdministratorExport
                    administratorId={balance.administrator_id ?? groupKeyOf(balance)}
                    label={groupLabelOf(balance)}
                    href={reportHref}
                    missingCost={lines.missingCost}
                    initialCounter={balance.export_counter ?? 0}
                  />
                ) : (
                  <ReportButton
                    href={reportHref}
                    missingCost={lines.missingCost}
                    label={`Gerar PDF de ${groupLabelOf(balance)}`}
                    iconOnly
                  />
                ))}
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type="button"
                    size="icon"
                    variant="outline"
                    className="h-7 w-7"
                    aria-label={`Ocultar ${groupLabelOf(balance)}`}
                    onClick={onHide}
                  >
                    <EyeOff className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  Tira esta linha da lista, dos totais e do PDF até recarregar a página.
                </TooltipContent>
              </Tooltip>
            </span>
          </span>
        </TableCell>
        {/* One administrator can run accounts on several programmes, so naming one of them
            here would be picking a winner. The column stays for alignment. */}
        <TableCell>
          {byAdministrator ? (
            <span className="text-muted-foreground">—</span>
          ) : (
            <span className="flex min-w-0 items-center gap-2">
              <span
                className="h-2.5 w-2.5 shrink-0 rounded-full border border-slate-300"
                style={{ backgroundColor: balance.loyalty_program_color }}
              />
              <span className="truncate">{balance.loyalty_program_name}</span>
            </span>
          )}
        </TableCell>
        <TableCell className={byAdministrator ? "tabular-nums" : undefined}>
          {byAdministrator
            ? (balance.account_count ?? 1)
            : (balance.administrator_name ?? "—")}
        </TableCell>
        <TableCell
          className={cn(
            "text-right font-medium tabular-nums",
            balance.net_points < 0 && "text-red-700",
          )}
        >
          {formatPoints(balance.net_points)}
          {balance.refunded_points > 0 && (
            <span className="block text-xs font-normal text-muted-foreground">
              {formatPoints(balance.issued_points)} − {formatPoints(balance.refunded_points)}
            </span>
          )}
        </TableCell>
        <TableCell className="text-right tabular-nums text-muted-foreground">
          {formatCurrencyBRL(lines.taxes)}
        </TableCell>
        <TableCell />
        <TableCell
          className={cn(
            "text-right font-medium tabular-nums",
            lines.amount < 0 && "text-red-700",
          )}
        >
          {formatCurrencyBRL(lines.amount)}
        </TableCell>

        {/* Three states rather than a checkbox: "algumas" is a real answer here and a
            two-state box would have to round it to one of the other two. Clicking settles
            the account either way, which is the move the operator actually wants. */}
        <TableCell className="text-center" onClick={(e) => e.stopPropagation()}>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="h-7 w-7"
            aria-label={
              allReviewed
                ? `Desmarcar as emissões de ${balance.account_label}`
                : `Marcar as emissões de ${balance.account_label} como conferidas`
            }
            onClick={() =>
              onCommit(
                balance.issuances.map((line) => line.issuance_id),
                { reviewed: !allReviewed },
                () => (allReviewed ? "Conferência desfeita." : "Emissões conferidas."),
              )
            }
          >
            {allReviewed ? (
              <CheckCheck className="h-4 w-4 text-emerald-700" />
            ) : someReviewed ? (
              <Minus className="h-4 w-4 text-amber-600" />
            ) : (
              <Square className="h-4 w-4 text-slate-300" />
            )}
          </Button>
        </TableCell>
      </TableRow>

      {isOpen &&
        balance.issuances.map((line, index) => (
          <SupplierIssuanceRow
            // An issuance appears twice when it was created and cancelled in the same
            // period - once as the booking, once as the credit - so the id alone is not
            // unique here.
            key={`${line.issuance_id}-${line.kind}-${index}`}
            line={line}
            showAccount={byAdministrator}
            saving={saving.has(line.issuance_id)}
            onCommit={onCommit}
          />
        ))}
    </>
  )
}

function SupplierIssuanceRow({
  line,
  showAccount,
  saving,
  onCommit,
}: {
  line: SupplierIssuance
  showAccount: boolean
  saving: boolean
  onCommit: (ids: string[], terms: PaymentTerms, describe: (n: number) => string) => void
}) {
  const amount = supplierLineAmount(line)
  const isRefund = line.kind === "refund"

  return (
    <TableRow className="bg-slate-50/60 text-sm">
      <TableCell />
      <TableCell className={cn("font-medium", isRefund && "text-red-700")}>
        <span className="flex items-center gap-1.5">
          <span className="flex flex-col">
            {line.program_locator}
            {showAccount && line.account_label && (
              <span className="text-xs font-normal text-muted-foreground">
                {line.account_label}
              </span>
            )}
          </span>
          {isRefund && (
            <span className="rounded-full bg-red-100 px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-red-800 uppercase">
              estorno
            </span>
          )}
        </span>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(line.date)}</TableCell>
      <TableCell className="text-muted-foreground">
        {line.origin_iata_code ?? "—"}
        {line.destination_iata_code ? ` → ${line.destination_iata_code}` : ""}
      </TableCell>
      <TableCell className={cn("text-right tabular-nums", isRefund && "text-red-700")}>
        {formatPoints(line.points)}
      </TableCell>

      {/* The figure and the tick that decides whether it counts sit together: the amount
          is shown either way, struck through when it is not being reimbursed, because a
          blank cell would read as "this booking had no taxes". */}
      <TableCell className="text-right">
        <label className="inline-flex cursor-pointer items-center justify-end gap-1.5">
          <span
            className={cn(
              "tabular-nums",
              line.taxes_included_in_cost
                ? "text-slate-700"
                : "text-slate-400 line-through decoration-slate-300",
            )}
          >
            {line.taxes === null ? "—" : formatCurrencyBRL(line.taxes)}
          </span>
          <input
            type="checkbox"
            checked={line.taxes_included_in_cost}
            disabled={saving}
            aria-label="Incluir as taxas desta emissão no valor"
            // The new value is read once, here, and closed over. Reading e.target.checked
            // inside the message callback instead would report the opposite: that callback
            // runs after the save resolves, and by then the controlled checkbox has already
            // re-rendered back to the old prop, so the DOM node reads as the value before
            // the click.
            onChange={(e) => {
              const included = e.target.checked
              onCommit([line.issuance_id], { taxesIncludedInCost: included }, () =>
                included ? "Taxas incluídas." : "Taxas retiradas.",
              )
            }}
            className="h-3.5 w-3.5 shrink-0 accent-slate-800"
          />
        </label>
      </TableCell>

      <TableCell className="text-right">
        {/* On an account that owns its points the rate is derived from the batches that
            paid for the booking, so it is shown rather than offered for editing - typing
            over it would be overwritten by the next recompute, and the backend refuses it
            anyway. The batches are edited in Estoque de pontos. */}
        {line.owns_point_inventory ? (
          <span
            className="tabular-nums text-slate-500"
            title="Vem do estoque de pontos da conta. Edite os lotes em Estoque de pontos."
          >
            {line.cost_of_the_thousand === null
              ? "—"
              : formatCurrencyBRL(line.cost_of_the_thousand)}
          </span>
        ) : (
          <SupplierCostCell
            value={line.cost_of_the_thousand}
            saving={saving}
            onCommit={(cents) =>
              onCommit([line.issuance_id], { costOfTheThousand: cents }, () =>
                "Milheiro de custo atualizado.",
              )
            }
          />
        )}
      </TableCell>
      <TableCell
        className={cn(
          "text-right font-medium tabular-nums",
          amount !== null && amount < 0 && "text-red-700",
        )}
      >
        {amount === null ? <span className="text-amber-700">—</span> : formatCurrencyBRL(amount)}
      </TableCell>

      <TableCell className="text-center">
        <input
          type="checkbox"
          checked={line.reviewed}
          disabled={saving}
          aria-label="Marcar esta emissão como conferida"
          // Same reason as the tax tick above: the new value is read once here and closed
          // over, because by the time the toast callback runs the controlled box has already
          // re-rendered back to the old prop.
          onChange={(e) => {
            const reviewed = e.target.checked
            onCommit([line.issuance_id], { reviewed }, () =>
              reviewed ? "Emissão conferida." : "Conferência desfeita.",
            )
          }}
          className="h-3.5 w-3.5 shrink-0 accent-emerald-700"
        />
      </TableCell>
    </TableRow>
  )
}
