"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Save, Sparkles, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { FieldError } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { createPromoRecipeSchema } from "@/schemas/promo"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import { createPromoRecipeAction } from "@/actions/promo"
import { RecipeFields } from "./recipe-fields"

type FormData = z.infer<typeof createPromoRecipeSchema.request>

export const emptyRecipeForm: FormData = {
  name: "",
  domain: "",
  loyalty_program_id: "",
  cpf_selector: "",
  checkbox_selector: "",
  submit_selector: "",
  success_pattern: "",
  failure_pattern: "",
  wait_seconds: "5",
  active: true,
  notes: "",
}

export function CreateRecipeForm({ programs }: { programs: LoyaltyProgram[] }) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(createPromoRecipeSchema.request),
    mode: "onTouched",
    defaultValues: emptyRecipeForm,
  })

  function handleToggle() {
    if (isOpen) form.reset()
    setIsOpen((prev) => !prev)
  }

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await createPromoRecipeAction(data)
      if (result.error) {
        form.setError("root", { message: result.error })
        return
      }
      form.reset()
      setIsOpen(false)
      router.refresh()
    })
  }

  return (
    <div className="flex flex-col gap-4">
      <Button
        type="button"
        onClick={handleToggle}
        className={cn(
          "h-full transition-colors duration-200",
          isOpen ? "bg-slate-500 hover:bg-slate-600" : "bg-slate-800 hover:bg-slate-900",
        )}
      >
        <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
          {isOpen ? <X className="h-4 w-4" /> : <Sparkles className="h-4 w-4" />}
          {isOpen ? "Cancelar Cadastro" : "Cadastrar Receita"}
        </div>
      </Button>

      <div
        className={cn(
          "grid transition-all duration-300 ease-in-out",
          isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}
      >
        <div className="overflow-hidden">
          <div className="rounded-md border border-slate-200 p-4">
            <form onSubmit={form.handleSubmit(onSubmit)}>
              <RecipeFields
                form={form}
                programs={programs}
                isPending={isPending}
                idPrefix="create-recipe"
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
    </div>
  )
}
