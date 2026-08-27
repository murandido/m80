"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { IdCard, Save, User, X } from "lucide-react"
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
import { formatCPF } from "@/lib/promo"
import { createLoyaltyDocumentSchema } from "@/schemas/promo"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { createLoyaltyDocumentAction } from "@/actions/promo"

type FormData = z.infer<typeof createLoyaltyDocumentSchema.request>

export function CreateDocumentForm({ programs }: { programs: LoyaltyProgram[] }) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(createLoyaltyDocumentSchema.request),
    mode: "onTouched",
    defaultValues: { name: "", cpf: "", loyalty_program_id: "", active: true },
  })

  function handleToggle() {
    if (isOpen) form.reset()
    setIsOpen((prev) => !prev)
  }

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await createLoyaltyDocumentAction(data)
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
          {isOpen ? <X className="h-4 w-4" /> : <IdCard className="h-4 w-4" />}
          {isOpen ? "Cancelar Cadastro" : "Cadastrar CPF"}
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
              <Field className="flex flex-col gap-5">
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="name"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="doc-name">
                          Nome <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="doc-name"
                            aria-invalid={fieldState.invalid}
                            placeholder="De quem é o CPF"
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <User className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                        {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                      </>
                    )}
                  />
                </div>

                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="cpf"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="doc-cpf">
                          CPF <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="doc-cpf"
                            inputMode="numeric"
                            aria-invalid={fieldState.invalid}
                            placeholder="000.000.000-00"
                            disabled={isPending}
                            onChange={(event) =>
                              field.onChange(formatCPF(event.target.value))
                            }
                          />
                          <InputGroupAddon align="inline-end">
                            <IdCard className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                        {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
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
                        <FieldLabel htmlFor="doc-program">
                          Programa de fidelidade <span className="text-destructive">*</span>
                        </FieldLabel>
                        <LoyaltyProgramCombobox
                          loyaltyPrograms={programs}
                          value={field.value}
                          onChange={field.onChange}
                          disabled={isPending}
                        />
                        <p className="text-xs text-slate-500">
                          O mesmo CPF pode ser cadastrado em mais de um programa.
                        </p>
                        {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                      </>
                    )}
                  />
                </div>

                <div className="flex items-center justify-between text-sm">
                  <div className="flex flex-col">
                    <FieldLabel htmlFor="doc-active">Ativo</FieldLabel>
                    <p className="text-xs text-slate-500">
                      Inativo, fica fora dos próximos cadastros.
                    </p>
                  </div>
                  <Controller
                    name="active"
                    control={form.control}
                    render={({ field }) => (
                      <Switch
                        id="doc-active"
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
    </div>
  )
}
