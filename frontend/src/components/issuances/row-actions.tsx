"use client"

import { RotateCcw, Trash2 } from "lucide-react"
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
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { useGridContext } from "@/components/issuances/grid-context"
import { CancelIssuanceButton } from "@/components/issuances/cancel-issuance-dialog"
import { IssuanceRow } from "@/components/issuances/types"

export function RowActions({ r, row }: { r: number; row: IssuanceRow }) {
  const { requestSaveActiveCell, onDeleteRow } = useGridContext()

  // A draft has no persisted id to cancel, and an already-cancelled issuance can't be
  // cancelled again (backend returns 422) - hide the button in both cases.
  const canCancel = !row.isDraft && row.status !== "cancelled"
  // The inverse: a persisted, cancelled row can be reverted back to active.
  const canRevert = !row.isDraft && row.status === "cancelled"

  return (
    <div className="flex items-center justify-end gap-0.5">
      {canCancel && <CancelIssuanceButton r={r} />}
      {canRevert && <RevertCancellationButton r={r} />}
      <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 opacity-0 transition-opacity group-hover/row:opacity-100"
          onClick={() => requestSaveActiveCell()}
        >
          <Trash2 className="h-3.5 w-3.5 text-destructive" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Deletar emissão</AlertDialogTitle>
          <AlertDialogDescription>
            Você realmente deseja deletar essa emissão?
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="border-red-300 text-red-600 hover:bg-red-50 hover:text-red-700">
            Não
          </AlertDialogCancel>
          <AlertDialogAction onClick={() => onDeleteRow(r)}>
            Sim, deletar
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// Reverting a cancellation takes no input, so a plain confirmation (not a form) is enough.
// Mirrors the delete button: the dialog closes on confirm and the toast reports the outcome.
function RevertCancellationButton({ r }: { r: number }) {
  const { onUncancelRow } = useGridContext()

  async function handleRevert() {
    const result = await onUncancelRow(r)
    if ("error" in result) {
      toast.error(result.error)
      return
    }
    toast.success("Cancelamento revertido.")
  }

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          title="Reverter cancelamento"
          className="h-6 w-6 opacity-0 transition-opacity group-hover/row:opacity-100"
        >
          <RotateCcw className="h-3.5 w-3.5 text-emerald-600" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Reverter cancelamento</AlertDialogTitle>
          <AlertDialogDescription>
            A emissão voltará a ficar ativa e editável, e o pagamento da venda vinculada
            será restaurado ao estado anterior ao cancelamento.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Voltar</AlertDialogCancel>
          <AlertDialogAction onClick={() => handleRevert()}>Reverter</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
