"use client"

import { useMemo, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Check, ChevronDown, ChevronRight, FileText, TriangleAlert, X } from "lucide-react"
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
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { formatPoints } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { SupplierBalance, SupplierIssuance } from "@/schemas/sale"

interface SalesSuppliersTableProps {
  balances: SupplierBalance[]
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
const COLUMN_COUNT = 8

type PaymentTerms = { costOfTheThousand?: number | null; taxesIncludedInCost?: boolean }

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("pt-BR")
}

function reportHref(query: string, accountID?: string): string {
  const params = new URLSearchParams(query)
  if (accountID) params.set("account_id", accountID)
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

export function SalesSuppliersTable({ balances, query }: SalesSuppliersTableProps) {
  const router = useRouter()
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [saving, setSaving] = useState<Set<string>>(new Set())
  const [bulkText, setBulkText] = useState("")
  const [pendingBulk, setPendingBulk] = useState<{ terms: PaymentTerms; summary: string } | null>(
    null,
  )
  const [, startTransition] = useTransition()

  // Summed in the browser rather than asked of the API: the table does not paginate, so
  // these are exactly the rows on screen - which is what "total do que está aparecendo"
  // means. A server-side total could disagree with the visible rows and nobody would know
  // which one to trust.
  const totals = useMemo(() => sumSupplierBalances(balances), [balances])

  const allIssuanceIds = useMemo(
    () => [...new Set(balances.flatMap((b) => b.issuances.map((i) => i.issuance_id)))],
    [balances],
  )

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
      <div className="flex justify-end">
        <ReportButton
          href={reportHref(query)}
          missingCost={totals.missingCost}
          label="Gerar PDF de todas"
        />
      </div>

      <div className="max-h-[calc(100vh-24rem)] overflow-auto rounded-md border">
        <Table>
          <TableHeader className="sticky top-0 z-10 bg-slate-100">
            <TableRow>
              <TableHead className="w-8" />
              <TableHead>Conta</TableHead>
              <TableHead>Programa</TableHead>
              <TableHead>Administrador</TableHead>
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
            </TableRow>
          </TableHeader>
          <TableBody>
            {balances.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={COLUMN_COUNT}
                  className="py-10 text-center text-sm text-muted-foreground"
                >
                  Nenhuma emissão no período selecionado.
                </TableCell>
              </TableRow>
            )}
            {balances.map((balance) => (
              <SupplierAccountRows
                key={balance.account_id}
                balance={balance}
                isOpen={expanded.has(balance.account_id)}
                onToggle={() => toggleExpanded(balance.account_id)}
                lines={sumSupplierLines(balance.issuances)}
                saving={saving}
                reportHref={reportHref(query, balance.account_id)}
                onCommit={commit}
              />
            ))}
          </TableBody>
          {balances.length > 0 && (
            <TableFooter className="sticky bottom-0 bg-slate-100">
              <TableRow>
                <TableCell colSpan={4} className="font-medium">
                  Total ({balances.length} {balances.length === 1 ? "conta" : "contas"})
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
  isOpen,
  onToggle,
  lines,
  saving,
  reportHref,
  onCommit,
}: {
  balance: SupplierBalance
  isOpen: boolean
  onToggle: () => void
  lines: ReturnType<typeof sumSupplierLines>
  saving: Set<string>
  reportHref: string
  onCommit: (ids: string[], terms: PaymentTerms, describe: (n: number) => string) => void
}) {
  return (
    <>
      <TableRow className="cursor-pointer" onClick={onToggle}>
        <TableCell className="w-8 text-muted-foreground">
          {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </TableCell>
        <TableCell className="font-medium">
          <span className="flex items-center gap-2">
            {balance.account_label}
            <span onClick={(e) => e.stopPropagation()}>
              <ReportButton
                href={reportHref}
                missingCost={lines.missingCost}
                label={`Gerar PDF de ${balance.account_label}`}
                iconOnly
              />
            </span>
          </span>
        </TableCell>
        <TableCell>
          <span className="flex min-w-0 items-center gap-2">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full border border-slate-300"
              style={{ backgroundColor: balance.loyalty_program_color }}
            />
            <span className="truncate">{balance.loyalty_program_name}</span>
          </span>
        </TableCell>
        <TableCell>{balance.administrator_name ?? "—"}</TableCell>
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
      </TableRow>

      {isOpen &&
        balance.issuances.map((line, index) => (
          <SupplierIssuanceRow
            // An issuance appears twice when it was created and cancelled in the same
            // period - once as the booking, once as the credit - so the id alone is not
            // unique here.
            key={`${line.issuance_id}-${line.kind}-${index}`}
            line={line}
            saving={saving.has(line.issuance_id)}
            onCommit={onCommit}
          />
        ))}
    </>
  )
}

function SupplierIssuanceRow({
  line,
  saving,
  onCommit,
}: {
  line: SupplierIssuance
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
          {line.program_locator}
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
        <SupplierCostCell
          value={line.cost_of_the_thousand}
          saving={saving}
          onCommit={(cents) =>
            onCommit([line.issuance_id], { costOfTheThousand: cents }, () =>
              "Milheiro de custo atualizado.",
            )
          }
        />
      </TableCell>
      <TableCell
        className={cn(
          "text-right font-medium tabular-nums",
          amount !== null && amount < 0 && "text-red-700",
        )}
      >
        {amount === null ? <span className="text-amber-700">—</span> : formatCurrencyBRL(amount)}
      </TableCell>
    </TableRow>
  )
}
