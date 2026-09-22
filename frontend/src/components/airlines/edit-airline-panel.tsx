"use client"

import { useEffect, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Building2, Hash, KeyRound, Tag } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { ColorPickerField } from "@/components/shared/color-picker-field"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { Airline, updateAirlineSchema } from "@/schemas/airline"
import { updateAirlineAction } from "@/actions/airline"

type FormData = z.infer<typeof updateAirlineSchema.request>

interface EditAirlinePanelProps {
  airline: Airline
  isOpen: boolean
  onSaved: () => void
}

export function EditAirlinePanel({ airline, isOpen, onSaved }: EditAirlinePanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const defaultValues: FormData = {
    name: airline.name,
    code: airline.code,
    traveos_supported: airline.traveos_supported,
    direct_booking_enabled: airline.direct_booking_enabled,
    requires_last_name: airline.requires_last_name,
    requires_purchase_number: airline.requires_purchase_number,
    traveos_airline_id: airline.traveos_airline_id?.toString() ?? "",
    brand_color: airline.brand_color ?? "",
  }

  const form = useForm<FormData>({
    resolver: zodResolver(updateAirlineSchema.request),
    mode: "onTouched",
    defaultValues,
  })

  // Re-seeded whenever the panel closes, and whenever the airline itself changes.
  // The second half matters after a save: the panel closes before router.refresh()
  // lands, so resetting only on close would put back the value the operator just
  // replaced, and reopening would look as if nothing had been saved.
  useEffect(() => {
    if (!isOpen) {
      form.reset(defaultValues)
    }
  }, [isOpen, airline]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateAirlineAction(airline.id, data)
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
              Informações da companhia aérea
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Veja e/ou atualize as informações da companhia aérea aqui!
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
                    value={airline.id}
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
                      <FieldLabel htmlFor="edit-code">
                        Código <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-code"
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
                          disabled={isPending}
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
                          disabled={isPending}
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

              {/* direct_booking_enabled */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="direct_booking_enabled"
                  control={form.control}
                  render={({ field }) => (
                    <>
                      <FieldLabel>Integração direta</FieldLabel>
                      <div className="flex gap-2">
                        <Button
                          type="button"
                          size="sm"
                          onClick={() => field.onChange(true)}
                          disabled={isPending}
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
                          disabled={isPending}
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
                          disabled={isPending}
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
                          disabled={isPending}
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
                          disabled={isPending}
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
                          disabled={isPending}
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

              {/* traveos_airline_id */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="traveos_airline_id"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="traveos_airline_id">ID na Traveos</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="traveos_airline_id"
                          type="text"
                          inputMode="numeric"
                          aria-invalid={fieldState.invalid}
                          placeholder="1"
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <Hash className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      <p className="text-muted-foreground text-xs">
                        Id da cia no catálogo da Traveos, exigido em toda emissão. Sem ele a
                        busca de dados é recusada, mesmo com &quot;Suporta Traveos&quot; ligado.
                      </p>
                      {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                    </>
                  )}
                />
              </div>

              {/* brand_color */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="brand_color"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="brand_color">Cor da marca</FieldLabel>
                      {/* The same picker the loyalty programs use. A hex field
                          alone asks somebody to know what #E30613 looks like. */}
                      <ColorPickerField
                        id="brand_color"
                        value={field.value}
                        onChange={field.onChange}
                        disabled={isPending}
                      />
                      <p className="text-muted-foreground text-xs">
                        Usada no bilhete. Formato #RRGGBB.
                      </p>
                      {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
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
