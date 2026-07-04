"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Building2, Plane, Tag, X, Save } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { createAirlineSchema } from "@/schemas/airline"
import { createAirlineAction } from "@/actions/airline"

type FormData = z.infer<typeof createAirlineSchema.request>

export function CreateAirlineForm() {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(createAirlineSchema.request),
    mode: "onTouched",
    defaultValues: {
      name: "",
      code: "",
      traveos_supported: false,
      requires_last_name: false,
      requires_purchase_number: false,
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
      const result = await createAirlineAction(data)

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
          {isOpen ? <X className="h-4 w-4" /> : <Plane className="h-4 w-4" />}
          {isOpen ? "Cancelar Cadastro" : "Cadastrar Companhia Aérea"}
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
                            placeholder="LATAM"
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Building2 className="text-foreground h-4 w-4" />
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
                            placeholder="LA"
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

                {/* traveos_supported */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="traveos_supported"
                    control={form.control}
                    render={({ field }) => (
                      <>
                        <FieldLabel>Suporta Traveos</FieldLabel>
                        <div className="flex gap-2">
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => field.onChange(true)}
                            className={cn(
                              "flex-1 transition-colors duration-150",
                              field.value
                                ? "bg-slate-800 text-white hover:bg-slate-900"
                                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
                            )}
                          >
                            Sim
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => field.onChange(false)}
                            className={cn(
                              "flex-1 transition-colors duration-150",
                              !field.value
                                ? "bg-slate-800 text-white hover:bg-slate-900"
                                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
                            )}
                          >
                            Não
                          </Button>
                        </div>
                      </>
                    )}
                  />
                </div>

                {/* requires_last_name */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="requires_last_name"
                    control={form.control}
                    render={({ field }) => (
                      <>
                        <FieldLabel>Exige sobrenome</FieldLabel>
                        <div className="flex gap-2">
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => field.onChange(true)}
                            className={cn(
                              "flex-1 transition-colors duration-150",
                              field.value
                                ? "bg-slate-800 text-white hover:bg-slate-900"
                                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
                            )}
                          >
                            Sim
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => field.onChange(false)}
                            className={cn(
                              "flex-1 transition-colors duration-150",
                              !field.value
                                ? "bg-slate-800 text-white hover:bg-slate-900"
                                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
                            )}
                          >
                            Não
                          </Button>
                        </div>
                      </>
                    )}
                  />
                </div>

                {/* requires_purchase_number */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="requires_purchase_number"
                    control={form.control}
                    render={({ field }) => (
                      <>
                        <FieldLabel>Exige número da compra</FieldLabel>
                        <div className="flex gap-2">
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => field.onChange(true)}
                            className={cn(
                              "flex-1 transition-colors duration-150",
                              field.value
                                ? "bg-slate-800 text-white hover:bg-slate-900"
                                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
                            )}
                          >
                            Sim
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => field.onChange(false)}
                            className={cn(
                              "flex-1 transition-colors duration-150",
                              !field.value
                                ? "bg-slate-800 text-white hover:bg-slate-900"
                                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
                            )}
                          >
                            Não
                          </Button>
                        </div>
                      </>
                    )}
                  />
                </div>

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
