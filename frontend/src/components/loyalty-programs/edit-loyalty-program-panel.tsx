"use client"

import { useEffect, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Award, Hash, Save, Tag } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { CPFLimitFields } from "./cpf-limit-fields"
import { RedemptionFeeFields } from "./redemption-fee-fields"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { LoyaltyProgram, updateLoyaltyProgramSchema } from "@/schemas/loyalty-program"
import { Airline } from "@/schemas/airline"
import { updateLoyaltyProgramAction } from "@/actions/loyalty-program"
import { AirlineCombobox } from "@/components/airlines/airline-combobox"
import { ColorPickerField } from "@/components/shared/color-picker-field"

type FormData = z.infer<typeof updateLoyaltyProgramSchema.request>

interface EditLoyaltyProgramPanelProps {
  loyaltyProgram: LoyaltyProgram
  airlines: Airline[]
  isOpen: boolean
  onSaved: () => void
}

// centsToInput renders a stored amount for a text field the operator types reais into.
// Null stays empty, which is how the form says the program charges no redemption fee -
// distinct from a fee of zero.
function centsToInput(cents: number | null | undefined): string {
  if (cents == null) return ""
  return (cents / 100).toFixed(2).replace(".", ",")
}

export function EditLoyaltyProgramPanel({
  loyaltyProgram,
  airlines,
  isOpen,
  onSaved,
}: EditLoyaltyProgramPanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const defaultValues: FormData = {
    name: loyaltyProgram.name,
    code: loyaltyProgram.code,
    color: loyaltyProgram.color,
    default_airline_id: loyaltyProgram.default_airline_id,
    cpf_limit: loyaltyProgram.cpf_limit != null ? String(loyaltyProgram.cpf_limit) : "",
    cpf_limit_window: loyaltyProgram.cpf_limit_window ?? "",
    cpf_limit_mode: loyaltyProgram.cpf_limit_mode ?? "",
    // Centavos on the way in, reais in the field: the action converts back on the way out.
    redemption_fee_domestic: centsToInput(loyaltyProgram.redemption_fee_domestic_cents),
    redemption_fee_international: centsToInput(
      loyaltyProgram.redemption_fee_international_cents,
    ),
    redemption_fee_advance_days:
      loyaltyProgram.redemption_fee_advance_days != null
        ? String(loyaltyProgram.redemption_fee_advance_days)
        : "",
  }

  const form = useForm<FormData>({
    resolver: zodResolver(updateLoyaltyProgramSchema.request),
    mode: "onTouched",
    defaultValues,
  })

  // Reset form whenever the panel closes
  useEffect(() => {
    if (!isOpen) {
      form.reset(defaultValues)
    }
  }, [isOpen]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateLoyaltyProgramAction(loyaltyProgram.id, data)
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
          <div className="mb-4">
            <h2 className="text-sm font-semibold text-slate-800">
              Informações do programa de fidelidade
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Veja e/ou atualize as informações do programa de fidelidade aqui!
            </p>
          </div>

          <form onSubmit={form.handleSubmit(onSubmit)}>
            <Field className="flex flex-col gap-5">
              {/* UUID — read-only */}
              <div className="flex flex-col gap-1 text-sm">
                <FieldLabel htmlFor="edit-uuid">UUID</FieldLabel>
                <InputGroup className="bg-slate-100">
                  <InputGroupInput
                    id="edit-uuid"
                    type="text"
                    value={loyaltyProgram.id}
                    readOnly
                    className="cursor-default select-all text-slate-400"
                  />
                  <InputGroupAddon align="inline-end">
                    <Hash className="h-4 w-4 text-slate-400" />
                  </InputGroupAddon>
                </InputGroup>
              </div>

              {/* name */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="name"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-name">
                        Nome <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-name"
                          type="text"
                          aria-invalid={fieldState.invalid}
                          placeholder="LATAM Pass"
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <Award className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* code */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="code"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-code">
                        Código <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-code"
                          type="text"
                          aria-invalid={fieldState.invalid}
                          placeholder="LP"
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <Tag className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* color */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="color"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-color">Cor</FieldLabel>
                      <ColorPickerField
                        id="edit-color"
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

              {/* default_airline_id */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="default_airline_id"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-default-airline-id">
                        Companhia aérea padrão <span className="text-destructive">*</span>
                      </FieldLabel>
                      <AirlineCombobox
                        airlines={airlines}
                        value={field.value}
                        onChange={field.onChange}
                        disabled={isPending}
                        placeholder="Buscar companhia aérea por nome ou código..."
                      />
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              <Separator />
              <CPFLimitFields
                control={form.control}
                disabled={isPending}
                idPrefix={`edit-${loyaltyProgram.id}`}
              />

              <Separator />
              <RedemptionFeeFields
                control={form.control}
                disabled={isPending}
                idPrefix={`edit-${loyaltyProgram.id}`}
              />

              <Separator />

              {form.formState.errors.root && (
                <p className="text-destructive text-sm">
                  {form.formState.errors.root.message}
                </p>
              )}

              <Button
                type="submit"
                className="h-full bg-slate-800 hover:bg-slate-900"
                disabled={isPending}
              >
                <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
                  {isPending ? <Spinner /> : <Save className="h-4 w-4" />}
                  {isPending ? "Salvando..." : "Salvar Alterações"}
                </div>
              </Button>
            </Field>
          </form>
        </div>
      </div>
    </div>
  )
}
