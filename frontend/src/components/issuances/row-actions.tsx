"use client"

import { useState } from "react"
import Link from "next/link"
import { CloudDownload, RotateCcw, Ticket, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
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
import { IssuanceRow, isRowSyncing } from "@/components/issuances/types"

export function RowActions({ r, row }: { r: number; row: IssuanceRow }) {
  const { requestSaveActiveCell, onDeleteRow } = useGridContext()

  // A draft has no persisted id to cancel, and an already-cancelled issuance can't be
  // cancelled again (backend returns 422) - hide the button in both cases.
  const canCancel = !row.isDraft && row.status !== "cancelled"
  // The inverse: a persisted, cancelled row can be reverted back to active.
  const canRevert = !row.isDraft && row.status === "cancelled"
  // Note what is NOT a condition here: traveos_synced_at. Pressing this on a row that
  // already has Traveos data is the point - it means "the flight changed, fetch it
  // again", which is the only route a reschedule has into the system. Cancelled rows
  // are excluded because there is nothing left to keep in sync.
  const canSyncWithTraveos = !row.isDraft && row.status !== "cancelled"
  // The ticket, on the other hand, IS gated on traveos_synced_at - and not on `complete`,
  // which anyone can tick on the grid. A ticket built from a hand-ticked row would have no
  // flights and no passengers: a blank document handed to a passenger.
  const canOpenTicket = !row.isDraft && row.traveos_synced_at !== null

  return (
    <div className="flex items-center justify-end gap-0.5">
      {canOpenTicket && <TicketButton row={row} />}
      {canSyncWithTraveos && <TraveosSyncButton r={r} row={row} />}
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

// Pulls the itinerary from Traveos. No confirmation dialog: on an unsynced row this is
// the expected next step, and on a synced one it overwrites data that came from the
// airline with fresher data from the same airline - neither is a decision worth a modal.
//
// It can run for minutes, because Traveos drives the airline's own website to issue the
// ticket. So unlike the other row actions, the button stays visible (not hover-only)
// while it works, and the toast is raised on the way in rather than only at the end.
function TraveosSyncButton({ r, row }: { r: number; row: IssuanceRow }) {
  const { onSyncRowWithTraveos, syncingRowKeys } = useGridContext()
  const [isPending, setIsPending] = useState(false)
  const isSyncing = isPending || isRowSyncing(row, syncingRowKeys)

  const alreadySynced = row.traveos_synced_at !== null

  async function handleSync() {
    if (isSyncing) return
    setIsPending(true)
    // Said up front because the wait is long enough that a silent button reads as
    // broken, and the operator may well walk away from it.
    toast.info(
      alreadySynced
        ? "Atualizando os dados do voo na Traveos. Isso pode levar alguns minutos."
        : "Puxando os dados na Traveos. Isso pode levar alguns minutos.",
    )
    try {
      const result = await onSyncRowWithTraveos(r)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Dados da Traveos atualizados.")
    } finally {
      setIsPending(false)
    }
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      disabled={isSyncing}
      title={
        alreadySynced
          ? "Atualizar dados do voo na Traveos"
          : "Puxar dados da Traveos"
      }
      onClick={() => void handleSync()}
      className={cn(
        "h-6 w-6 transition-opacity",
        // Hover-only like its neighbours, except while it runs: hiding a button that
        // is still working would make the operator wonder whether they pressed it.
        isSyncing ? "opacity-100" : "opacity-0 group-hover/row:opacity-100",
      )}
    >
      {isSyncing ? (
        <Spinner className="h-3.5 w-3.5" />
      ) : (
        <CloudDownload
          className={cn("h-3.5 w-3.5", alreadySynced ? "text-slate-500" : "text-sky-600")}
        />
      )}
    </Button>
  )
}

// Opens the ticket page, which is where the PDF download lives too. A link and not an
// action: nothing is mutated, and a real anchor is what lets the operator open it in a
// new tab, which is what they do when checking one booking against another.
function TicketButton({ row }: { row: IssuanceRow }) {
  return (
    <Button
      asChild
      variant="ghost"
      size="icon"
      title="Ver bilhete"
      className="h-6 w-6 opacity-0 transition-opacity group-hover/row:opacity-100"
    >
      {/* row.id, not row.key: a row created in this session keeps its draft key. */}
      <Link href={`/dashboard/issuances/${row.id}/ticket`}>
        <Ticket className="h-3.5 w-3.5 text-violet-600" />
      </Link>
    </Button>
  )
}
