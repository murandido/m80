"use client"

import { useEffect, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Save, User } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"
import {
  updateLoyaltyDocumentSchema,
  type LoyaltyDocument,
} from "@/schemas/promo"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import { updateLoyaltyDocumentAction } from "@/actions/promo"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"

type FormData = z.infer<typeof updateLoyaltyDocumentSchema.request>

interface EditDocumentPanelProps {
  document: LoyaltyDocument
  programs: LoyaltyProgram[]
  isOpen: boolean
  onSaved: () => void
}

function toDocumentFormValues(document: LoyaltyDocument): FormData {
  return {
    name: document.name,
    // Sent back unchanged: the update request is a whole object, so leaving it out
    // would fail validation. Same payload the active Switch on the card already
    // sends.
    cpf: document.cpf,
    loyalty_program_id: document.loyalty_program_id,
    active: document.active,
  }
}

// Everything about a registered CPF except the CPF itself.
//
// The number is deliberately not editable. The enrolment history in promo_enrollments
// points at this record, so rewriting the document would silently make that history
// describe a different person. A CPF typed wrong is fixed by deleting the record and
// creating it again, which now works even with history behind it - the delete takes
// that history with it, which is the honest version of the same correction.
export function EditDocumentPanel({
  document,
  programs,
  isOpen,
  onSaved,
}: EditDocumentPanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(updateLoyaltyDocumentSchema.request),
    mode: "onTouched",
    defaultValues: toDocumentFormValues(document),
  })

  // Re-seeded whenever the panel closes, and whenever the record changes while closed.
  // It used to be seeded once on mount: flipping the card's "ativo" switch and then
  // saving a name edit here sent the old flag back and silently undid the switch.
  useEffect(() => {
    if (!isOpen) {
      form.reset(toDocumentFormValues(document))
    }
  }, [isOpen, document]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateLoyaltyDocumentAction(document.id, data)
      if (result.error) {
        form.setError("root", { message: result.error })
        return
      }
      onSaved()
      router.refresh()
    })
  }

  const idPrefix = `edit-doc-${document.id}`

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
            <Field className="flex flex-col gap-5">
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="name"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor={`${idPrefix}-name`}>
                        Nome <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id={`${idPrefix}-name`}
                          aria-invalid={fieldState.invalid}
                          placeholder="De quem é o CPF"
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <User className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="loyalty_program_id"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor={`${idPrefix}-program`}>
                        Programa de fidelidade{" "}
                        <span className="text-destructive">*</span>
                      </FieldLabel>
                      <LoyaltyProgramCombobox
                        loyaltyPrograms={programs}
                        value={field.value}
                        onChange={field.onChange}
                        disabled={isPending}
                      />
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              <div className="flex items-center justify-between text-sm">
                <div className="flex flex-col">
                  <FieldLabel htmlFor={`${idPrefix}-active`}>Ativo</FieldLabel>
                  <p className="text-xs text-slate-500">
                    Inativo, fica fora dos próximos cadastros.
                  </p>
                </div>
                <Controller
                  name="active"
                  control={form.control}
                  render={({ field }) => (
                    <Switch
                      id={`${idPrefix}-active`}
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isPending}
                    />
                  )}
                />
              </div>
            </Field>

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
