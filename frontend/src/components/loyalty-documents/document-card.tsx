"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Eye, EyeOff, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"
import { formatCPF, maskCPF } from "@/lib/promo"
import type { LoyaltyDocument } from "@/schemas/promo"
import {
  deleteLoyaltyDocumentAction,
  updateLoyaltyDocumentAction,
} from "@/actions/promo"

// The CPF is masked until someone asks for it. It is stored encrypted and the panel is
// allowed to show it - staff need the number to reconcile an account - but a screen
// full of bare documents is not something to leave open on a monitor.
export function DocumentCard({ document }: { document: LoyaltyDocument }) {
  const router = useRouter()
  const [isRevealed, setIsRevealed] = useState(false)
  const [isActive, setIsActive] = useState(document.active)
  const [isDeleted, setIsDeleted] = useState(false)
  const [isPending, startTransition] = useTransition()

  function handleToggleActive(next: boolean) {
    setIsActive(next)
    startTransition(async () => {
      const result = await updateLoyaltyDocumentAction(document.id, {
        name: document.name,
        cpf: document.cpf,
        loyalty_program_id: document.loyalty_program_id,
        active: next,
      })
      if (result.error) {
        setIsActive(!next)
        toast.error(result.error)
        return
      }
      router.refresh()
    })
  }

  function handleDelete() {
    startTransition(async () => {
      const result = await deleteLoyaltyDocumentAction(document.id)
      if (result.error) {
        // The API answers 422 telling the operator to deactivate when the document
        // already has enrolment history - passing that sentence through is the point.
        toast.error(result.error)
        return
      }
      setIsDeleted(true)
    })
  }

  return (
    <div
      className={cn(
        "flex items-center justify-between rounded-md border border-slate-200 px-4 py-3 transition-opacity duration-300",
        isDeleted ? "opacity-0" : "opacity-100",
      )}
      onTransitionEnd={() => {
        if (isDeleted) router.refresh()
      }}
    >
      <div className="flex flex-col gap-0.5">
        <div className="flex items-baseline gap-2">
          <span className="text-sm font-medium text-slate-800">{document.name}</span>
          <span className="font-mono text-xs text-slate-500">
            {isRevealed ? formatCPF(document.cpf) : maskCPF(document.cpf)}
          </span>
        </div>
        <span className="text-xs text-slate-500">
          {document.loyalty_program_name || "Programa não identificado"}
        </span>
      </div>

      <div className="flex items-center gap-3">
        <Switch
          checked={isActive}
          onCheckedChange={handleToggleActive}
          disabled={isPending}
          aria-label="Ativo"
        />
        <Button
          variant="outline"
          size="icon"
          title={isRevealed ? "Ocultar CPF" : "Mostrar CPF"}
          onClick={() => setIsRevealed((prev) => !prev)}
          className="border-slate-800 text-slate-800"
        >
          {isRevealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </Button>
        <Button
          variant="outline"
          size="icon"
          title="Excluir"
          onClick={handleDelete}
          disabled={isPending}
          className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
