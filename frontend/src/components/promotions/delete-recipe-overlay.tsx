"use client"

import { useState, useTransition } from "react"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { deletePromoRecipeAction } from "@/actions/promo"

interface DeleteRecipeOverlayProps {
  recipeName: string
  recipeId: string
  isOpen: boolean
  onDismissed: () => void
  onDeleted: () => void
}

export function DeleteRecipeOverlay({
  recipeName,
  recipeId,
  isOpen,
  onDismissed,
  onDeleted,
}: DeleteRecipeOverlayProps) {
  const [isDismissing, setIsDismissing] = useState(false)
  const [noTransition, setNoTransition] = useState(false)
  const [isPending, startTransition] = useTransition()

  const translateClass = isDismissing
    ? "-translate-x-full"
    : isOpen
      ? "translate-x-0"
      : "translate-x-full"

  function handleConfirm() {
    startTransition(async () => {
      const result = await deletePromoRecipeAction(recipeId)
      if (!result.error) onDeleted()
    })
  }

  function handleTransitionEnd() {
    if (isDismissing) {
      setNoTransition(true)
      setIsDismissing(false)
      onDismissed()
      requestAnimationFrame(() =>
        requestAnimationFrame(() => setNoTransition(false)),
      )
    }
  }

  return (
    <div
      className={cn(
        "absolute inset-0 flex items-center justify-between bg-red-600 px-4 ease-in-out",
        noTransition ? "" : "transition-transform duration-300",
        translateClass,
      )}
      onTransitionEnd={handleTransitionEnd}
    >
      <span className="text-sm font-medium text-white">
        Deseja excluir &apos;{recipeName}&apos;?
      </span>
      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={handleConfirm}
          disabled={isPending}
          className="rounded-md bg-white text-slate-800 hover:bg-slate-100 active:bg-slate-200"
        >
          {isPending ? <Spinner className="h-4 w-4" /> : "Sim"}
        </Button>
        <Button
          size="sm"
          onClick={() => setIsDismissing(true)}
          disabled={isPending}
          className="rounded-md bg-white text-slate-800 hover:bg-slate-100 active:bg-slate-200"
        >
          Não
        </Button>
      </div>
    </div>
  )
}
