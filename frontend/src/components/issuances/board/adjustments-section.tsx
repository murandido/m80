"use client"

import { useState } from "react"
import { Pencil, Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
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
import { cn } from "@/lib/utils"
import { formatCurrencyBRL } from "@/lib/currency"
import { formatSignedPaddedPoints } from "@/lib/points"
import { AdjustmentDialog } from "@/components/issuances/board/adjustment-dialog"
import type { IssuanceBoardState } from "@/components/issuances/board/use-issuance-board"
import type { IssuanceRow } from "@/components/issuances/types"
import type { IssuanceAdjustment } from "@/schemas/issuance"

function formatDay(isoDay: string): string {
  const [year, month, day] = isoDay.split("-")
  return `${day}/${month}/${year}`
}

function signedPoints(points: number): string {
  const formatted = formatSignedPaddedPoints(Math.abs(points))
  return `${points < 0 ? "−" : "+"}${formatted} pts`
}

function signedMoney(cents: number): string {
  return `${cents < 0 ? "−" : "+"}${formatCurrencyBRL(Math.abs(cents))}`
}

// What happened to the ticket after it was issued, one line per alteration. Kept apart from
// "Valores da emissão" on purpose: those fields are the booking as it was made, and editing
// them is correcting a typo. An alteration is a new fact with its own date.
export function AdjustmentsSection({
  board,
  row,
}: {
  board: IssuanceBoardState
  row: IssuanceRow
}) {
  const [editing, setEditing] = useState<IssuanceAdjustment | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [deleting, setDeleting] = useState<IssuanceAdjustment | null>(null)
  const cancelled = row.status === "cancelled"

  function openFor(adjustment: IssuanceAdjustment | null) {
    setEditing(adjustment)
    setDialogOpen(true)
  }

  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-xl font-semibold text-slate-500 sm:text-2xl">Alterações</h2>
        {!cancelled && (
          <Button type="button" variant="outline" size="sm" onClick={() => openFor(null)}>
            <Plus />
            Registrar alteração
          </Button>
        )}
      </div>

      {row.adjustments.length === 0 ? (
        <p className="text-sm text-slate-500">
          {cancelled
            ? "Nenhuma alteração registrada."
            : "Nenhuma alteração. Registre aqui quando a companhia cobrar pontos ou taxas a mais por uma mudança na passagem, ou devolver algo."}
        </p>
      ) : (
        <ul className="flex flex-col divide-y divide-slate-200 rounded-lg bg-white">
          {row.adjustments.map((adjustment) => {
            const refund = adjustment.points < 0 || (adjustment.points === 0 && adjustment.taxes < 0)
            return (
              <li key={adjustment.id} className="flex items-start gap-3 px-3 py-2.5 text-sm">
                <span
                  className={cn(
                    "mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-xs font-medium",
                    refund ? "bg-emerald-100 text-emerald-800" : "bg-amber-100 text-amber-900",
                  )}
                >
                  {refund ? "Reembolso" : "Cobrança extra"}
                </span>
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="flex flex-wrap gap-x-2 tabular-nums text-slate-900">
                    <span className="text-slate-500">{formatDay(adjustment.occurred_on)}</span>
                    {adjustment.points !== 0 && <span>{signedPoints(adjustment.points)}</span>}
                    {adjustment.taxes !== 0 && <span>{signedMoney(adjustment.taxes)} taxas</span>}
                  </span>
                  <span className="text-xs text-slate-500">
                    {adjustment.client_amount_cents === 0
                      ? refund
                        ? "Mantido pela agência"
                        : "Absorvido pela agência"
                      : `${adjustment.client_amount_cents > 0 ? "Cobrado do cliente" : "Devolvido ao cliente"}: ${formatCurrencyBRL(Math.abs(adjustment.client_amount_cents))}`}
                  </span>
                  <span className="break-words text-slate-700">{adjustment.reason}</span>
                </div>
                {!cancelled && (
                  <div className="flex shrink-0 items-center">
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
                      title="Corrigir alteração"
                      onClick={() => openFor(adjustment)}
                    >
                      <Pencil className="h-4 w-4 text-slate-500" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
                      title="Excluir alteração"
                      onClick={() => setDeleting(adjustment)}
                    >
                      <Trash2 className="h-4 w-4 text-red-600" />
                    </Button>
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}

      <AdjustmentDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        adjustment={editing}
        issuanceRate={row.value_of_the_thousand}
        onSave={(input) => board.saveAdjustment(row.key, editing?.id ?? null, input)}
      />

      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Excluir alteração</AlertDialogTitle>
            <AlertDialogDescription>
              Use só para uma alteração registrada por engano. A venda e o custeio voltam a
              não contar com ela.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Voltar</AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                if (!deleting) return
                const result = await board.deleteAdjustment(row.key, deleting.id)
                if ("error" in result) toast.error(result.error)
                else toast.success("Alteração excluída.")
                setDeleting(null)
              }}
            >
              Excluir
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}
