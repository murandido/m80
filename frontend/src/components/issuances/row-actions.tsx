"use client"

import { Trash2 } from "lucide-react"
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

export function RowActions({ r }: { r: number }) {
  const { requestSaveActiveCell, onDeleteRow } = useGridContext()

  return (
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
  )
}
