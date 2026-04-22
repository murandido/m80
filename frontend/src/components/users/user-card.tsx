"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Menu, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { User } from "@/schemas/user"
import { DeleteOverlay } from "./delete-overlay"
import { EditUserPanel } from "./edit-user-panel"

interface UserCardProps {
  user: User
  isCurrentUser?: boolean
}

export function UserCard({ user, isCurrentUser = false }: UserCardProps) {
  const router = useRouter()
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  const [isEditOpen, setIsEditOpen] = useState(false)
  const [isDeleted, setIsDeleted] = useState(false)

  function handleCardFadeEnd() {
    if (isDeleted) router.refresh()
  }

  return (
    <div className="flex flex-col">
      <div
        className={cn(
          "relative overflow-hidden border border-slate-200 transition-opacity duration-300",
          isEditOpen ? "rounded-t-md" : "rounded-md",
          isDeleted ? "opacity-0" : "opacity-100",
        )}
        onTransitionEnd={handleCardFadeEnd}
      >
        <div className="flex items-center justify-between px-4 py-3">
          <span className="text-sm font-medium text-slate-800">{user.name}</span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="icon"
              onClick={() => setIsDeleteOpen(true)}
              disabled={isCurrentUser}
              className={cn(
                "transition-colors duration-200",
                isCurrentUser
                  ? "cursor-not-allowed border-slate-300 text-slate-300"
                  : "border-slate-800 text-slate-800 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white",
              )}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              onClick={() => setIsEditOpen((prev) => !prev)}
              className={cn(
                "transition-colors duration-200",
                isEditOpen
                  ? "border-cyan-600 bg-cyan-600 text-white hover:border-cyan-700 hover:bg-cyan-700"
                  : "border-slate-800 text-slate-800 hover:border-cyan-400 hover:bg-cyan-400 hover:text-white active:border-cyan-800 active:bg-cyan-800 active:text-white",
              )}
            >
              <Menu className="h-4 w-4" />
            </Button>
          </div>
        </div>

        <DeleteOverlay
          userName={user.name}
          userId={user.id}
          isOpen={isDeleteOpen}
          onDismissed={() => setIsDeleteOpen(false)}
          onDeleted={() => setIsDeleted(true)}
        />
      </div>

      <EditUserPanel
        user={user}
        isOpen={isEditOpen}
        onSaved={() => setIsEditOpen(false)}
      />
    </div>
  )
}
