"use client"

import { useEffect, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { deletePersonAction, previewDeletePersonAction } from "@/actions/person"
import { PersonRemoval } from "@/schemas/person"

interface DeleteHolderPanelProps {
  holderId: string
  holderName: string
  isOpen: boolean
  onCancel: () => void
}

function plural(count: number, one: string, many: string): string {
  return count === 1 ? `1 ${one}` : `${count} ${many}`
}

// What the delete is about to do, in the operator's words. Nothing here is a guess: it is
// the backend's own dry run (GET /people/{id}/delete-preview).
function describe(removal: PersonRemoval): string[] {
  const lines: string[] = []
  if (removal.accounts_deleted > 0) {
    lines.push(
      `${plural(removal.accounts_deleted, "conta sem histórico é apagada", "contas sem histórico são apagadas")} de vez.`,
    )
  }
  if (removal.accounts_archived > 0) {
    lines.push(
      `${plural(removal.accounts_archived, "conta com histórico é arquivada", "contas com histórico são arquivadas")}: some do painel, e as emissões antigas continuam mostrando a conta.`,
    )
  }
  lines.push(
    removal.person_deleted
      ? "O titular é apagado de vez."
      : "O titular é arquivado (ainda há histórico com o nome dele) e os apelidos dele ficam livres para o bot.",
  )
  return lines
}

// Deleting a row: the holder and every account of theirs, deleted where nothing refers to
// them and archived otherwise (backend internal/account/remove.go). This is what finally
// clears the rows the bot created from a mistyped message.
export function DeleteHolderPanel({
  holderId,
  holderName,
  isOpen,
  onCancel,
}: DeleteHolderPanelProps) {
  const router = useRouter()
  const [removal, setRemoval] = useState<PersonRemoval | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  useEffect(() => {
    if (!isOpen) return
    let current = true
    previewDeletePersonAction(holderId).then((result) => {
      if (!current) return
      if (result.error) {
        setError(result.error)
        setRemoval(null)
        return
      }
      setError(null)
      setRemoval(result.removal ?? null)
    })
    return () => {
      current = false
    }
  }, [isOpen, holderId])

  function handleConfirm() {
    startTransition(async () => {
      const result = await deletePersonAction(holderId)
      if ("error" in result) {
        setError(result.error ?? "Erro ao excluir.")
        return
      }
      router.refresh()
    })
  }

  return (
    <div
      className={cn(
        "grid transition-all duration-300 ease-in-out",
        isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
    >
      <div className="overflow-hidden">
        <div className="flex flex-col gap-3 rounded-b-md border border-t-0 border-red-200 bg-red-50 p-4 text-sm">
          <h2 className="font-semibold text-red-900">
            Excluir “{holderName}”?
          </h2>

          {!removal && !error && <Spinner />}
          {removal && (
            <ul className="list-disc pl-5 text-red-900">
              {describe(removal).map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          )}
          {error && <p className="text-destructive">{error}</p>}

          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={onCancel}
              disabled={isPending}
            >
              Cancelar
            </Button>
            <Button
              type="button"
              onClick={handleConfirm}
              disabled={isPending || !removal}
              className="bg-red-600 hover:bg-red-700"
            >
              {isPending ? <Spinner /> : <Trash2 className="h-4 w-4" />}
              Excluir
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
