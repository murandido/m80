"use client"

import { useEffect, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import {
  User as UserIcon,
  Hash,
  Mail,
  PhoneCall,
  MapPin,
  Landmark,
  Building2,
  Home,
  Signpost,
  FileText,
  KeyRound,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { DateInput } from "@/components/ui/date-input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { Person, updatePersonSchema } from "@/schemas/person"
import { updatePersonAction } from "@/actions/person"

type FormData = z.infer<typeof updatePersonSchema.request>

interface EditPersonPanelProps {
  person: Person
  isOpen: boolean
  onSaved: () => void
}

function toFormValues(person: Person): FormData {
  return {
    name: person.name ?? "",
    cpf: person.cpf ?? "",
    // Response comes back as an ISO datetime string; <input type="date"> needs "YYYY-MM-DD".
    birth_date: person.birth_date ? person.birth_date.slice(0, 10) : "",
    email: person.email ?? "",
    phone: person.phone ?? "",
    zip_code: person.zip_code ?? "",
    state: person.state ?? "",
    city: person.city ?? "",
    address: person.address ?? "",
    address_number: person.address_number ?? "",
    complement: person.complement ?? "",
    neighborhood: person.neighborhood ?? "",
  }
}

export function EditPersonPanel({ person, isOpen, onSaved }: EditPersonPanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(updatePersonSchema.request),
    mode: "onTouched",
    defaultValues: toFormValues(person),
  })

  // Re-seeded whenever the panel closes, and whenever the record itself changes
  // while closed. The second half matters after a save or a merge: the panel closes
  // before router.refresh() lands, so resetting only on close would reopen showing
  // the pre-save values - and saving that form again would write them back.
  useEffect(() => {
    if (!isOpen) {
      form.reset(toFormValues(person))
    }
  }, [isOpen, person]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updatePersonAction(person.id, data)
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
              Informações do titular
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Veja e/ou atualize as informações do titular aqui!
            </p>
          </div>

          <form onSubmit={form.handleSubmit(onSubmit)}>
            <Field className="flex flex-col gap-5">
              <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                Dados pessoais
              </h3>

              {/* name */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="name"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-person-name">Nome</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-person-name"
                          type="text"
                          aria-invalid={fieldState.invalid}
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <UserIcon className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* cpf */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="cpf"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-person-cpf">CPF</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-person-cpf"
                          type="text"
                          aria-invalid={fieldState.invalid}
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <Hash className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* birth_date */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="birth_date"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-person-birth-date">
                        Data de nascimento
                      </FieldLabel>
                      <DateInput
                        id="edit-person-birth-date"
                        value={field.value ?? ""}
                        onChange={field.onChange}
                        aria-invalid={fieldState.invalid}
                        disabled={isPending}
                        className="[&_input]:bg-slate-200"
                      />
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* email */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="email"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-person-email">Email</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-person-email"
                          type="email"
                          aria-invalid={fieldState.invalid}
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <Mail className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* phone */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="phone"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-person-phone">Telefone</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-person-phone"
                          type="tel"
                          aria-invalid={fieldState.invalid}
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <PhoneCall className="text-foreground h-4 w-4" />
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
              <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                Endereço
              </h3>

              {/* zip_code */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="zip_code"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-person-zip">CEP</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-person-zip"
                          type="text"
                          aria-invalid={fieldState.invalid}
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

              {/* state + city */}
              <div className="grid grid-cols-2 gap-3">
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="state"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="edit-person-state">Estado</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="edit-person-state"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Landmark className="text-foreground h-4 w-4" />
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
                    name="city"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="edit-person-city">Cidade</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="edit-person-city"
                            type="text"
                            aria-invalid={fieldState.invalid}
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
              </div>

              {/* address + number */}
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2 flex flex-col gap-1 text-sm">
                  <Controller
                    name="address"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="edit-person-address">Endereço</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="edit-person-address"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Home className="text-foreground h-4 w-4" />
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
                    name="address_number"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="edit-person-address-number">
                          Número
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="edit-person-address-number"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            disabled={isPending}
                          />
                        </InputGroup>
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>
              </div>

              {/* complement + neighborhood */}
              <div className="grid grid-cols-2 gap-3">
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="complement"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="edit-person-complement">
                          Complemento
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="edit-person-complement"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <FileText className="text-foreground h-4 w-4" />
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
                    name="neighborhood"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="edit-person-neighborhood">
                          Bairro
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="edit-person-neighborhood"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Signpost className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>
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
