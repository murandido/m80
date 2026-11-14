"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Award, Save, Tag, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { CPFLimitFields } from "./cpf-limit-fields"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { createLoyaltyProgramSchema } from "@/schemas/loyalty-program"
import { Airline } from "@/schemas/airline"
import { createLoyaltyProgramAction } from "@/actions/loyalty-program"
import { AirlineCombobox } from "@/components/airlines/airline-combobox"
import { ColorPickerField } from "@/components/shared/color-picker-field"

type FormData = z.infer<typeof createLoyaltyProgramSchema.request>

interface CreateLoyaltyProgramFormProps {
  airlines: Airline[]
}

export function CreateLoyaltyProgramForm({ airlines }: CreateLoyaltyProgramFormProps) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(createLoyaltyProgramSchema.request),
    mode: "onTouched",
    defaultValues: {
      name: "",
      code: "",
      color: "#64748B",
      default_airline_id: "",
      cpf_limit: "",
      cpf_limit_window: "",
      cpf_limit_mode: "",
    },
  })

  function handleToggle() {
    if (isOpen) {
      form.reset()
    }
    setIsOpen((prev) => !prev)
  }

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await createLoyaltyProgramAction(data)

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
      {/* toggle button */}
      <Button
        type="button"
        onClick={handleToggle}
        className={cn(
          "h-full transition-colors duration-200",
          isOpen ? "bg-slate-500 hover:bg-slate-600" : "bg-slate-800 hover:bg-slate-900",
        )}
      >
        <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
          {isOpen ? <X className="h-4 w-4" /> : <Award className="h-4 w-4" />}
          {isOpen ? "Cancelar Cadastro" : "Cadastrar Programa de Fidelidade"}
        </div>
      </Button>

      {/* animated form panel */}
      <div
        className={cn(
          "grid transition-all duration-300 ease-in-out",
          isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}
      >
        <div className="overflow-hidden">
          <div className="rounded-md border border-slate-200 p-4">
            <form onSubmit={form.handleSubmit(onSubmit)}>
              <Field className="flex flex-col gap-5">
                {/* name */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="name"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="name">
                          Nome <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="name"
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
                        <FieldLabel htmlFor="code">
                          Código <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="code"
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
                        <FieldLabel htmlFor="color">Cor</FieldLabel>
                        <ColorPickerField
                          id="color"
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
                        <FieldLabel htmlFor="default_airline_id">
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
                <CPFLimitFields control={form.control} disabled={isPending} idPrefix="create" />


                <Separator />

                {/* root error */}
                {form.formState.errors.root && (
                  <p className="text-destructive text-sm">
                    {form.formState.errors.root.message}
                  </p>
                )}

                {/* submit */}
                <Button
                  type="submit"
                  className="h-full bg-slate-800 hover:bg-slate-900"
                  disabled={isPending}
                >
                  <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
                    {isPending ? <Spinner /> : <Save className="h-4 w-4" />}
                    {isPending ? "Salvando..." : "Salvar"}
                  </div>
                </Button>
              </Field>
            </form>
          </div>
        </div>
      </div>
    </div>
  )
}
