"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Building2, Globe, Map, MapPin, Plane, Save, Tag, X } from "lucide-react"
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
import { createAirportSchema } from "@/schemas/airport"
import { createAirportAction } from "@/actions/airport"

type FormData = z.infer<typeof createAirportSchema.request>

export function CreateAirportForm() {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(createAirportSchema.request),
    mode: "onTouched",
    defaultValues: {
      iata_code: "",
      name: "",
      city: "",
      state: "",
      country: "BR",
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
      const result = await createAirportAction(data)

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
          {isOpen ? "Cancelar Cadastro" : "Cadastrar Aeroporto"}
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
                {/* iata_code */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="iata_code"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="iata_code">
                          Código IATA <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="iata_code"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            placeholder="GRU"
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
                            placeholder="Aeroporto Internacional de Guarulhos"
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <MapPin className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>

                {/* city */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="city"
                    control={form.control}
                    render={({ field }) => (
                      <>
                        <FieldLabel htmlFor="city">Cidade</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="city"
                            type="text"
                            placeholder="Guarulhos"
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Building2 className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                      </>
                    )}
                  />
                </div>

                {/* state */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="state"
                    control={form.control}
                    render={({ field }) => (
                      <>
                        <FieldLabel htmlFor="state">Estado</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="state"
                            type="text"
                            placeholder="SP"
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Map className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                      </>
                    )}
                  />
                </div>

                {/* country */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="country"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="country">
                          País <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="country"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            placeholder="BR"
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Globe className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
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
