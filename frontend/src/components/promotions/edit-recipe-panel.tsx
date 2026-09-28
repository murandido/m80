"use client"

import { useEffect, useTransition } from "react"
import { useRouter } from "next/navigation"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Save } from "lucide-react"
import { Button } from "@/components/ui/button"
import { FieldError } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { updatePromoRecipeSchema, type PromoRecipe } from "@/schemas/promo"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import { updatePromoRecipeAction } from "@/actions/promo"
import { RecipeFields } from "./recipe-fields"

type FormData = z.infer<typeof updatePromoRecipeSchema.request>

interface EditRecipePanelProps {
  recipe: PromoRecipe
  programs: LoyaltyProgram[]
  isOpen: boolean
  onSaved: () => void
}

function toRecipeFormValues(recipe: PromoRecipe): FormData {
  return {
    name: recipe.name,
    domain: recipe.domain,
    loyalty_program_id: recipe.loyalty_program_id,
    // Null comes back from the API as "not pinned"; the form shows it as blank, and
    // blanking a field is how a selector is handed back to the heuristic.
    cpf_selector: recipe.cpf_selector ?? "",
    checkbox_selector: recipe.checkbox_selector ?? "",
    submit_selector: recipe.submit_selector ?? "",
    success_pattern: recipe.success_pattern ?? "",
    failure_pattern: recipe.failure_pattern ?? "",
    wait_seconds: String(recipe.wait_seconds),
    active: recipe.active,
    notes: recipe.notes ?? "",
  }
}

export function EditRecipePanel({
  recipe,
  programs,
  isOpen,
  onSaved,
}: EditRecipePanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(updatePromoRecipeSchema.request),
    mode: "onTouched",
    defaultValues: toRecipeFormValues(recipe),
  })

  // Re-seeded whenever the panel closes, and whenever the record changes while closed.
  // It used to be seeded once on mount: flipping the card's "ativo" switch and then
  // saving a name edit here sent the old flag back and silently undid the switch.
  useEffect(() => {
    if (!isOpen) {
      form.reset(toRecipeFormValues(recipe))
    }
  }, [isOpen, recipe]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updatePromoRecipeAction(recipe.id, data)
      if (result.error) {
        form.setError("root", { message: result.error })
        return
      }
      onSaved()
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
        <div className="rounded-b-md border border-t-0 border-slate-200 p-4">
          <form onSubmit={form.handleSubmit(onSubmit)}>
            <RecipeFields
              form={form}
              programs={programs}
              isPending={isPending}
              idPrefix={`edit-recipe-${recipe.id}`}
            />

            {form.formState.errors.root && (
              <div className="mt-4">
                <FieldError errors={[form.formState.errors.root]} />
              </div>
            )}

            <Button
              type="submit"
              disabled={isPending}
              className="mt-5 w-full bg-slate-800 hover:bg-slate-900"
            >
              {isPending ? (
                <Spinner className="h-4 w-4" />
              ) : (
                <span className="flex items-center gap-2 text-sm">
                  <Save className="h-4 w-4" />
                  Salvar
                </span>
              )}
            </Button>
          </form>
        </div>
      </div>
    </div>
  )
}
