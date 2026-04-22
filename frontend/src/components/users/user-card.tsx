"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Trash2, Menu } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { User } from "@/schemas/user"
import { deleteUserAction } from "@/actions/user"

type Status = "idle" | "confirming" | "dismissing" | "deleting" | "deleted"

interface UserCardProps {
  user: User
}

const overlayTranslate: Record<Status, string> = {
  idle: "translate-x-full",
  confirming: "translate-x-0",
  dismissing: "-translate-x-full",
  deleting: "translate-x-0",
  deleted: "translate-x-0",
}

export function UserCard({ user }: UserCardProps) {
  const router = useRouter()
  const [status, setStatus] = useState<Status>("idle")
  const [noTransition, setNoTransition] = useState(false)
  const [isPending, startTransition] = useTransition()

  function handleDismiss() {
    setStatus("dismissing")
  }

  function handleConfirm() {
    setStatus("deleting")
    startTransition(async () => {
      const result = await deleteUserAction(user.id)
      if (result.error) {
        setStatus("confirming")
        return
      }
      setStatus("deleted")
    })
  }

  function handleOverlayTransitionEnd() {
    if (status === "dismissing") {
      // Disable transition so the snap from off-screen-left back to
      // off-screen-right is invisible, then re-enable after two frames.
      setNoTransition(true)
      setStatus("idle")
      requestAnimationFrame(() =>
        requestAnimationFrame(() => setNoTransition(false)),
      )
    }
  }

  function handleCardTransitionEnd() {
    if (status === "deleted") {
      router.refresh()
    }
  }

  return (
    <div
      className={cn(
        "relative overflow-hidden rounded-md border border-slate-200 transition-opacity duration-300",
        status === "deleted" ? "opacity-0" : "opacity-100",
      )}
      onTransitionEnd={handleCardTransitionEnd}
    >
      {/* card content */}
      <div className="flex items-center justify-between px-4 py-3">
        <span className="text-sm font-medium text-slate-800">{user.name}</span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="icon"
            onClick={() => setStatus("confirming")}
            className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-cyan-400 hover:bg-cyan-400 hover:text-white active:border-cyan-800 active:bg-cyan-800 active:text-white"
          >
            <Menu className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* confirmation overlay */}
      <div
        className={cn(
          "absolute inset-0 flex items-center justify-between bg-red-600 px-4 ease-in-out",
          noTransition ? "" : "transition-transform duration-300",
          overlayTranslate[status],
        )}
        onTransitionEnd={handleOverlayTransitionEnd}
      >
        <span className="text-sm font-medium text-white">
          Deseja deletar &apos;{user.name}&apos;?
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
            onClick={handleDismiss}
            disabled={isPending}
            className="rounded-md bg-white text-slate-800 hover:bg-slate-100 active:bg-slate-200"
          >
            Não
          </Button>
        </div>
      </div>
    </div>
  )
}
