"use client"

import { CheckSquare, Minus, Square } from "lucide-react"
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatCurrencyBRL } from "@/lib/currency"
import { formatPoints } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { Agency } from "@/schemas/agency"
import type {
  LegacyImportBlock,
  LegacyImportRow,
  LegacyImportVerdict,
} from "@/schemas/legacy-import"

// A row is identified by its block and its line in the sheet - the same pair the commit
// sends back, and the only thing the server accepts as a selection.
export function rowKey(blockIndex: number, number: number): string {
  return `${blockIndex}:${number}`
}

// Only a row that fully resolved can be written; everything else the server would skip
// anyway, so it is shown with its reason instead of a checkbox that would lie.
export function isSelectable(row: LegacyImportRow): boolean {
  return row.verdict === "ok"
}

const VERDICT_LABEL: Record<Exclude<LegacyImportVerdict, "ok">, string> = {
  already_imported: "Já no sistema",
  needs_mapping: "Falta mapear",
  duplicate_in_file: "Repetida na planilha",
  invalid: "Não dá para importar",
}

interface LegacyImportPreviewTableProps {
  blocks: LegacyImportBlock[]
  // Keys the operator unticked by hand. Kept as the exception rather than as the
  // selection itself, so re-resolving after a mapping does not undo their choices.
  deselected: Set<string>
  onToggleRow: (key: string) => void
  onToggleBlock: (block: LegacyImportBlock, select: boolean) => void
  agencies: Agency[]
  // Per-row agency, keyed by rowKey. A row with no entry follows the day's agency,
  // which is what defaultAgencyId carries.
  rowAgencies: Record<string, string>
  defaultAgencyId: string
  onRowAgency: (key: string, agencyId: string) => void
  disabled?: boolean
}

export function LegacyImportPreviewTable({
  blocks,
  deselected,
  onToggleRow,
  onToggleBlock,
  agencies,
  rowAgencies,
  defaultAgencyId,
  onRowAgency,
  disabled,
}: LegacyImportPreviewTableProps) {
  return (
    <div className="space-y-6">
      {/* The template ships six blocks per day whether or not anyone used them, and an
          empty one has nothing to decide about. */}
      {blocks
        .filter((block) => (block.rows ?? []).length > 0)
        .map((block) => {
          const rows = block.rows ?? []
          const selectable = rows.filter(isSelectable)
          const selected = selectable.filter(
            (row) => !deselected.has(rowKey(block.index, row.number)),
          )
          const allSelected =
            selectable.length > 0 && selected.length === selectable.length
          const noneSelected = selected.length === 0

          return (
            <div key={block.index} className="rounded-md border">
              {/* The sheet groups by operator and so does the decision to bring a block
                in - Clara and Wilmara already register their days through the panel. */}
              <div className="flex items-center gap-3 border-b bg-slate-50 px-3 py-2">
                <button
                  type="button"
                  onClick={() => onToggleBlock(block, !allSelected)}
                  disabled={selectable.length === 0}
                  className="flex items-center gap-2 text-sm font-medium disabled:opacity-50"
                  aria-label={`Marcar ou desmarcar o bloco de ${block.issuer_name}`}
                >
                  {allSelected ? (
                    <CheckSquare className="h-4 w-4" />
                  ) : noneSelected ? (
                    <Square className="h-4 w-4" />
                  ) : (
                    <Minus className="h-4 w-4" />
                  )}
                  {block.issuer_name || "Sem emissor no título"}
                </button>
                <span className="text-muted-foreground text-xs">
                  {selected.length} de {rows.length} linha
                  {rows.length === 1 ? "" : "s"}
                </span>
              </div>

              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10" />
                      <TableHead className="w-14">Linha</TableHead>
                      <TableHead>Cia</TableHead>
                      <TableHead>Localizador</TableHead>
                      <TableHead>Voo</TableHead>
                      <TableHead>Passageiro</TableHead>
                      <TableHead>Trecho</TableHead>
                      <TableHead className="text-right">Pontos</TableHead>
                      <TableHead className="text-right">Milheiro</TableHead>
                      <TableHead className="text-right">Taxa</TableHead>
                      <TableHead>Conta</TableHead>
                      <TableHead className="w-52">Agência</TableHead>
                      <TableHead>Situação</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.map((row) => {
                      const key = rowKey(block.index, row.number)
                      const checked = isSelectable(row) && !deselected.has(key)

                      return (
                        <TableRow
                          key={key}
                          className={cn(
                            !isSelectable(row) &&
                              "text-muted-foreground bg-slate-50",
                          )}
                        >
                          <TableCell>
                            <button
                              type="button"
                              onClick={() => onToggleRow(key)}
                              disabled={!isSelectable(row)}
                              className="disabled:opacity-40"
                              aria-label={`Marcar ou desmarcar a linha ${row.number}`}
                            >
                              {checked ? (
                                <CheckSquare className="h-4 w-4" />
                              ) : (
                                <Square className="h-4 w-4" />
                              )}
                            </button>
                          </TableCell>
                          <TableCell className="text-xs tabular-nums">
                            {row.number}
                          </TableCell>
                          <TableCell className="text-xs">
                            {row.loyalty_program_name}
                          </TableCell>
                          <TableCell className="font-mono text-xs">
                            {row.program_locator}
                            {row.airline_locator && (
                              <span className="text-muted-foreground">
                                {" "}
                                ({row.airline_locator})
                              </span>
                            )}
                          </TableCell>
                          <TableCell className="text-xs">
                            {formatDate(row.flight_date)}
                          </TableCell>
                          <TableCell className="max-w-40 truncate text-xs">
                            {row.main_passenger_name || "—"}
                          </TableCell>
                          <TableCell className="text-xs">
                            {row.origin_iata_code || "—"} →{" "}
                            {row.destination_iata_code || "—"}
                          </TableCell>
                          <TableCell className="text-right text-xs tabular-nums">
                            {formatPoints(row.points)}
                          </TableCell>
                          <TableCell className="text-right text-xs tabular-nums">
                            {formatCurrencyBRL(row.value_of_the_thousand)}
                          </TableCell>
                          <TableCell className="text-right text-xs tabular-nums">
                            {row.taxes == null
                              ? "—"
                              : formatCurrencyBRL(row.taxes)}
                          </TableCell>
                          <TableCell className="max-w-44 text-xs">
                            {/* Both spellings: the alias as the sheet has it, and the
                              account it reached. Checking one against the other is
                              the point of the preview. */}
                            <span className="block truncate">
                              {row.account_name || "—"}
                            </span>
                            <span className="text-muted-foreground block truncate text-[11px]">
                              planilha: {row.account_label || "—"}
                            </span>
                          </TableCell>
                          <TableCell className="w-52">
                            {/* Which agency an issuance belongs to is a fact about
                              that issuance, and the spreadsheet has no column for it -
                              so it is asked here, row by row, seeded from the day's. */}
                            <AgencyCombobox
                              agencies={agencies}
                              value={rowAgencies[key] ?? defaultAgencyId}
                              onChange={(agencyId) =>
                                onRowAgency(key, agencyId)
                              }
                              clearable={false}
                              disabled={disabled || !isSelectable(row)}
                            />
                          </TableCell>
                          <TableCell className="max-w-52 text-xs">
                            {row.verdict === "ok" ? (
                              <Badge variant="secondary">Vai importar</Badge>
                            ) : (
                              <span className="flex flex-col gap-0.5">
                                <Badge variant="outline">
                                  {VERDICT_LABEL[row.verdict]}
                                </Badge>
                                <span className="text-[11px]">
                                  {row.reason}
                                </span>
                              </span>
                            )}
                            {(row.notes ?? []).map((note) => (
                              <span
                                key={note}
                                className="mt-0.5 block text-[11px] text-amber-600"
                              >
                                {note}
                              </span>
                            ))}
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>
            </div>
          )
        })}
    </div>
  )
}

// The API sends an ISO timestamp; the operator reads DD/MM/AAAA, the same as everywhere
// else in the panel.
function formatDate(value: string | null | undefined): string {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "—"
  return date.toLocaleDateString("pt-BR", { timeZone: "UTC" })
}
