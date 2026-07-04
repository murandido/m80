"use client"

import { useState, useTransition } from "react"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { deleteAccountAction } from "@/actions/account"

interface DeleteOverlayProps {
  accountName: string
  accountId: string
  isOpen: boolean
  onDismissed: () => void
  onDeleted: () => void
}

export function DeleteOverlay({
  accountName,
  accountId,
  isOpen,
  onDismissed,
  onDeleted,
}: DeleteOverlayProps) {
  const [isDismissing, setIsDismissing] = useState(false)
  const [noTransition, setNoTransition] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  const translateClass = isDismissing
    ? "-translate-x-full"
    : isOpen
      ? "translate-x-0"
      : "translate-x-full"

  function handleDismiss() {
    setError(null)
    setIsDismissing(true)
  }

  function handleConfirm() {
    startTransition(async () => {
      const result = await deleteAccountAction(accountId)
      if (result.error) {
        // e.g. 422 "Não é possível excluir conta vinculada a uma emissão"
        setError(result.error)
        return
      }
      onDeleted()
    })
  }

  function handleTransitionEnd() {
    if (isDismissing) {
      // Disable transition so the snap from off-screen-left back to
      // off-screen-right is invisible, then re-enable after two frames.
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
        "absolute inset-0 flex items-center justify-between gap-2 bg-red-600 px-4 ease-in-out",
        noTransition ? "" : "transition-transform duration-300",
        translateClass,
      )}
      onTransitionEnd={handleTransitionEnd}
    >
      <span className="text-sm font-medium text-white">
        {error ? error : `Deseja deletar '${accountName}'?`}
      </span>
      <div className="flex shrink-0 gap-2">
        {!error && (
          <Button
            size="sm"
            onClick={handleConfirm}
            disabled={isPending}
            className="rounded-md bg-white text-slate-800 hover:bg-slate-100 active:bg-slate-200"
          >
            {isPending ? <Spinner className="h-4 w-4" /> : "Sim"}
          </Button>
        )}
        <Button
          size="sm"
          onClick={handleDismiss}
          disabled={isPending}
          className="rounded-md bg-white text-slate-800 hover:bg-slate-100 active:bg-slate-200"
        >
          {error ? "Fechar" : "Não"}
        </Button>
      </div>
    </div>
  )
}
