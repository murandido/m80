"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { FlaskConical, Menu, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import type { PromoRecipe } from "@/schemas/promo"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import { DeleteRecipeOverlay } from "./delete-recipe-overlay"
import { EditRecipePanel } from "./edit-recipe-panel"
import { TestRecipePanel } from "./test-recipe-panel"

interface RecipeCardProps {
  recipe: PromoRecipe
  programs: LoyaltyProgram[]
}

export function RecipeCard({ recipe, programs }: RecipeCardProps) {
  const router = useRouter()
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  const [isEditOpen, setIsEditOpen] = useState(false)
  const [isTestOpen, setIsTestOpen] = useState(false)
  const [isDeleted, setIsDeleted] = useState(false)

  const hasPanelOpen = isEditOpen || isTestOpen

  function handleCardFadeEnd() {
    if (isDeleted) router.refresh()
  }

  return (
    <div className="flex flex-col">
      <div
        className={cn(
          "relative overflow-hidden border border-slate-200 transition-opacity duration-300",
          hasPanelOpen ? "rounded-t-md" : "rounded-md",
          isDeleted ? "opacity-0" : "opacity-100",
        )}
        onTransitionEnd={handleCardFadeEnd}
      >
        <div className="flex items-center justify-between px-4 py-3">
          <div className="flex flex-col gap-0.5">
            <div className="flex items-baseline gap-2">
              <span className="text-sm font-medium text-slate-800">{recipe.name}</span>
              <span className="text-xs text-slate-500">{recipe.domain}</span>
              {!recipe.active && (
                <span className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-600">
                  Inativa
                </span>
              )}
            </div>
            <span className="text-xs text-slate-500">
              {recipe.loyalty_program_name || "Programa não identificado"}
              {" · "}
              {recipe.cpf_selector || recipe.submit_selector
                ? "seletores fixos"
                : "descoberta automática"}
            </span>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="icon"
              title="Testar em uma página"
              onClick={() => {
                setIsTestOpen((prev) => !prev)
                setIsEditOpen(false)
              }}
              className={cn(
                "transition-colors duration-200",
                isTestOpen
                  ? "border-emerald-600 bg-emerald-600 text-white hover:border-emerald-700 hover:bg-emerald-700"
                  : "border-slate-800 text-slate-800 hover:border-emerald-400 hover:bg-emerald-400 hover:text-white",
              )}
            >
              <FlaskConical className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              title="Excluir"
              onClick={() => setIsDeleteOpen(true)}
              className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              title="Editar"
              onClick={() => {
                setIsEditOpen((prev) => !prev)
                setIsTestOpen(false)
              }}
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

        <DeleteRecipeOverlay
          recipeName={recipe.name}
          recipeId={recipe.id}
          isOpen={isDeleteOpen}
          onDismissed={() => setIsDeleteOpen(false)}
          onDeleted={() => setIsDeleted(true)}
        />
      </div>

      <TestRecipePanel recipe={recipe} isOpen={isTestOpen} />

      <EditRecipePanel
        recipe={recipe}
        programs={programs}
        isOpen={isEditOpen}
        onSaved={() => setIsEditOpen(false)}
      />
    </div>
  )
}
