"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Menu, Merge, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { Person } from "@/schemas/person"
import { DeleteOverlay } from "./delete-overlay"
import { EditPersonPanel } from "./edit-person-panel"
import { MergePersonPanel } from "./merge-person-panel"

interface PersonCardProps {
  person: Person
  // Every owner, for the merge picker. Empty for anyone who cannot merge, so the list
  // is not shipped to browsers that have no use for it.
  people: Person[]
  canMerge: boolean
}

export function PersonCard({ person, people, canMerge }: PersonCardProps) {
  const router = useRouter()
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  // One panel at a time: they share the card's bottom edge, and two open at once would
  // both claim the rounded corner.
  const [openPanel, setOpenPanel] = useState<"none" | "edit" | "merge">("none")
  const [isDeleted, setIsDeleted] = useState(false)

  const isEditOpen = openPanel === "edit"
  const isMergeOpen = openPanel === "merge"

  function handleCardFadeEnd() {
    if (isDeleted) router.refresh()
  }

  const subtitle = [person.email, person.phone, person.cpf]
    .filter(Boolean)
    .join(" · ")

  return (
    <div className="flex flex-col">
      <div
        className={cn(
          "relative overflow-hidden border border-slate-200 transition-opacity duration-300",
          openPanel === "none" ? "rounded-md" : "rounded-t-md",
          isDeleted ? "opacity-0" : "opacity-100",
        )}
        onTransitionEnd={handleCardFadeEnd}
      >
        <div className="flex items-center justify-between px-4 py-3">
          <div className="flex flex-col">
            <span className="text-sm font-medium text-slate-800">{person.name}</span>
            {subtitle && (
              <span className="text-xs text-slate-500">{subtitle}</span>
            )}
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="icon"
              onClick={() => setIsDeleteOpen(true)}
              className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
            {canMerge && (
              <Button
                variant="outline"
                size="icon"
                onClick={() =>
                  setOpenPanel((prev) => (prev === "merge" ? "none" : "merge"))
                }
                className={cn(
                  "transition-colors duration-200",
                  isMergeOpen
                    ? "border-amber-600 bg-amber-600 text-white hover:border-amber-700 hover:bg-amber-700"
                    : "border-slate-800 text-slate-800 hover:border-amber-400 hover:bg-amber-400 hover:text-white active:border-amber-800 active:bg-amber-800 active:text-white",
                )}
              >
                <Merge className="h-4 w-4" />
              </Button>
            )}
            <Button
              variant="outline"
              size="icon"
              onClick={() =>
                setOpenPanel((prev) => (prev === "edit" ? "none" : "edit"))
              }
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
          personName={person.name}
          personId={person.id}
          isOpen={isDeleteOpen}
          onDismissed={() => setIsDeleteOpen(false)}
          onDeleted={() => setIsDeleted(true)}
        />
      </div>

      <EditPersonPanel
        person={person}
        isOpen={isEditOpen}
        onSaved={() => setOpenPanel("none")}
      />

      {canMerge && (
        <MergePersonPanel
          person={person}
          people={people}
          isOpen={isMergeOpen}
          onMerged={() => setOpenPanel("none")}
        />
      )}
    </div>
  )
}
