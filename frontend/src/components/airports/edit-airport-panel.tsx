"use client"

import { useEffect, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Building2, Globe, Hash, KeyRound, Map, MapPin, Tag } from "lucide-react"
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
import { Airport, updateAirportSchema } from "@/schemas/airport"
import { updateAirportAction } from "@/actions/airport"

type FormData = z.infer<typeof updateAirportSchema.request>

interface EditAirportPanelProps {
  airport: Airport
  isOpen: boolean
  onSaved: () => void
}

export function EditAirportPanel({ airport, isOpen, onSaved }: EditAirportPanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const defaultValues: FormData = {
    iata_code: airport.iata_code,
    name: airport.name,
    city: airport.city ?? "",
    state: airport.state ?? "",
    country: airport.country,
  }

  const form = useForm<FormData>({
    resolver: zodResolver(updateAirportSchema.request),
    mode: "onTouched",
    defaultValues,
  })

  // Re-seeded whenever the panel closes, and whenever the record itself changes
  // while closed. The second half matters after a save or a merge: the panel closes
  // before router.refresh() lands, so resetting only on close would reopen showing
  // the pre-save values - and saving that form again would write them back.
  useEffect(() => {
    if (!isOpen) {
      form.reset(defaultValues)
    }
  }, [isOpen, airport]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateAirportAction(airport.id, data)
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
              Informações do aeroporto
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Veja e/ou atualize as informações do aeroporto aqui!
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
                    value={airport.id}
                    readOnly
                    className="cursor-default select-all text-slate-400"
                  />
                  <InputGroupAddon align="inline-end">
                    <Hash className="h-4 w-4 text-slate-400" />
                  </InputGroupAddon>
                </InputGroup>
              </div>

              {/* iata_code */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="iata_code"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-iata_code">
                        Código IATA <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-iata_code"
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
                      <FieldLabel htmlFor="edit-name">
                        Nome <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-name"
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
                      <FieldLabel htmlFor="edit-city">Cidade</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-city"
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
                      <FieldLabel htmlFor="edit-state">Estado</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-state"
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
                      <FieldLabel htmlFor="edit-country">
                        País <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-country"
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
                  {isPending ? <Spinner /> : <KeyRound className="h-4 w-4" />}
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
