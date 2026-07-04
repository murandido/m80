"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import {
  UserPlus,
  X,
  User,
  Hash,
  Calendar,
  Mail,
  PhoneCall,
  MapPin,
  Landmark,
  Building2,
  Home,
  Signpost,
  FileText,
  Save,
} from "lucide-react"
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
import { createPersonSchema } from "@/schemas/person"
import { createPersonAction } from "@/actions/person"

type FormData = z.infer<typeof createPersonSchema.request>

export function CreatePersonForm() {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(createPersonSchema.request),
    mode: "onTouched",
    defaultValues: {
      name: "",
      cpf: "",
      birth_date: "",
      email: "",
      phone: "",
      zip_code: "",
      state: "",
      city: "",
      address: "",
      address_number: "",
      complement: "",
      neighborhood: "",
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
      const result = await createPersonAction(data)

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
          {isOpen ? <X className="h-4 w-4" /> : <UserPlus className="h-4 w-4" />}
          {isOpen ? "Cancelar Cadastro" : "Cadastrar Pessoa"}
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
                        <FieldLabel htmlFor="name">
                          Nome <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="name"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            placeholder="João Silva"
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

                {/* cpf */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="cpf"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="cpf">CPF</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="cpf"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            placeholder="000.000.000-00"
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
                        <FieldLabel htmlFor="birth_date">Data de nascimento</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="birth_date"
                            type="date"
                            aria-invalid={fieldState.invalid}
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <Calendar className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
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
                        <FieldLabel htmlFor="email">Email</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="email"
                            type="email"
                            aria-invalid={fieldState.invalid}
                            placeholder="joao@email.com"
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
                        <FieldLabel htmlFor="phone">Telefone</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="phone"
                            type="tel"
                            aria-invalid={fieldState.invalid}
                            placeholder="11999999999"
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
                        <FieldLabel htmlFor="zip_code">CEP</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="zip_code"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            placeholder="00000-000"
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
                          <FieldLabel htmlFor="state">Estado</FieldLabel>
                          <InputGroup className="bg-slate-200">
                            <InputGroupInput
                              {...field}
                              id="state"
                              type="text"
                              aria-invalid={fieldState.invalid}
                              placeholder="SP"
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
                          <FieldLabel htmlFor="city">Cidade</FieldLabel>
                          <InputGroup className="bg-slate-200">
                            <InputGroupInput
                              {...field}
                              id="city"
                              type="text"
                              aria-invalid={fieldState.invalid}
                              placeholder="São Paulo"
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
                          <FieldLabel htmlFor="address">Endereço</FieldLabel>
                          <InputGroup className="bg-slate-200">
                            <InputGroupInput
                              {...field}
                              id="address"
                              type="text"
                              aria-invalid={fieldState.invalid}
                              placeholder="Rua das Flores"
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
                          <FieldLabel htmlFor="address_number">Número</FieldLabel>
                          <InputGroup className="bg-slate-200">
                            <InputGroupInput
                              {...field}
                              id="address_number"
                              type="text"
                              aria-invalid={fieldState.invalid}
                              placeholder="123"
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
                          <FieldLabel htmlFor="complement">Complemento</FieldLabel>
                          <InputGroup className="bg-slate-200">
                            <InputGroupInput
                              {...field}
                              id="complement"
                              type="text"
                              aria-invalid={fieldState.invalid}
                              placeholder="Apto 12"
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
                          <FieldLabel htmlFor="neighborhood">Bairro</FieldLabel>
                          <InputGroup className="bg-slate-200">
                            <InputGroupInput
                              {...field}
                              id="neighborhood"
                              type="text"
                              aria-invalid={fieldState.invalid}
                              placeholder="Centro"
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
