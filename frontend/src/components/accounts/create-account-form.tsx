"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { KeyRound, Save, User, UserPlus, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { ACCOUNT_CATEGORY_OPTIONS } from "@/lib/account-category"
import { createAccountSchema } from "@/schemas/account"
import { PersonOption } from "@/schemas/person"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { createAccountAction } from "@/actions/account"
import { PersonCombobox } from "@/components/people/person-combobox"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { AccountNicknamesField } from "./account-nicknames-field"

type FormData = z.infer<typeof createAccountSchema.request>

interface CreateAccountFormProps {
  people: PersonOption[]
  loyaltyPrograms: LoyaltyProgram[]
}

export function CreateAccountForm({ people, loyaltyPrograms }: CreateAccountFormProps) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()
  // Guards against creating the account twice: only set when the account was
  // already created but some nicknames failed (rare race), keeping the panel
  // open so the user can see the warning instead of silently losing it.
  const [createdAccountId, setCreatedAccountId] = useState<string | null>(null)

  const form = useForm<FormData>({
    resolver: zodResolver(createAccountSchema.request),
    mode: "onTouched",
    defaultValues: {
      loyalty_program_id: "",
      login: "",
      password: "",
      holder_id: "",
      administrator_id: "",
      category: "unidentified" as const,
      waives_redemption_fee: false,
      nicknames: [],
    },
  })

  const loyaltyProgramId = form.watch("loyalty_program_id")

  function handleToggle() {
    if (isOpen) {
      form.reset()
      setCreatedAccountId(null)
    }
    setIsOpen((prev) => !prev)
  }

  function onSubmit(data: FormData) {
    if (createdAccountId) return

    startTransition(async () => {
      const result = await createAccountAction(data)

      if (result.error) {
        form.setError("root", { message: result.error })
        return
      }

      router.refresh()

      if (result.failedNicknames?.length) {
        setCreatedAccountId(result.accountId)
        form.setError("root", {
          message: `Conta criada. Apelido(s) não adicionado(s) por já estar(em) em uso: ${result.failedNicknames.join(", ")}`,
        })
        return
      }

      form.reset()
      setIsOpen(false)
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
          {isOpen ? "Cancelar Cadastro" : "Cadastrar Conta"}
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
                  Dados da conta
                </h3>

                {/* loyalty_program_id */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="loyalty_program_id"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="loyalty_program_id">
                          Programa de fidelidade{" "}
                          <span className="text-destructive">*</span>
                        </FieldLabel>
                        <LoyaltyProgramCombobox
                          loyaltyPrograms={loyaltyPrograms}
                          value={field.value}
                          onChange={field.onChange}
                          disabled={isPending}
                          placeholder="Buscar programa de fidelidade..."
                        />
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>

                {/* login */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="login"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="login">Login</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="login"
                            type="text"
                            aria-invalid={fieldState.invalid}
                            placeholder="usuario123"
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

                {/* password */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="password"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="password">Senha</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="password"
                            type="password"
                            aria-invalid={fieldState.invalid}
                            disabled={isPending}
                          />
                          <InputGroupAddon align="inline-end">
                            <KeyRound className="text-foreground h-4 w-4" />
                          </InputGroupAddon>
                        </InputGroup>
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>

                {/* category */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="category"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="category">Categoria</FieldLabel>
                        <Select
                          value={field.value ?? "unidentified"}
                          onValueChange={field.onChange}
                          disabled={isPending}
                        >
                          <SelectTrigger id="category" className="bg-slate-200">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {ACCOUNT_CATEGORY_OPTIONS.map((option) => (
                              <SelectItem key={option.value} value={option.value}>
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <p className="text-xs text-slate-500">
                          O controle de CPFs só vale em contas normais.
                        </p>
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>

                {/* waives_redemption_fee */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="waives_redemption_fee"
                    control={form.control}
                    render={({ field }) => (
                      <>
                        <div className="flex items-center justify-between gap-3">
                          <FieldLabel htmlFor="waives-redemption-fee">
                            Isenta de taxa de resgate
                          </FieldLabel>
                          <Switch
                            id="waives-redemption-fee"
                            checked={field.value ?? false}
                            onCheckedChange={field.onChange}
                            disabled={isPending}
                          />
                        </div>
                        <p className="text-xs text-slate-500">
                          É o caso das contas Azul Diamante Unique.
                        </p>
                      </>
                    )}
                  />
                </div>

                <Separator />
                <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                  Responsáveis
                </h3>

                {/* holder_id */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="holder_id"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="holder_id">Titular</FieldLabel>
                        <PersonCombobox
                          people={people}
                          value={field.value ?? ""}
                          onChange={field.onChange}
                          disabled={isPending}
                          placeholder="Buscar titular por nome ou apelido..."
                        />
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>

                {/* administrator_id */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="administrator_id"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="administrator_id">Administrador</FieldLabel>
                        <PersonCombobox
                          people={people}
                          value={field.value ?? ""}
                          onChange={field.onChange}
                          disabled={isPending}
                          placeholder="Buscar administrador por nome ou apelido..."
                        />
                        {fieldState.invalid && (
                          <FieldError errors={[fieldState.error]} />
                        )}
                      </>
                    )}
                  />
                </div>

                <Separator />
                <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                  Apelidos
                </h3>

                <Controller
                  name="nicknames"
                  control={form.control}
                  render={({ field }) => (
                    <AccountNicknamesField
                      value={field.value ?? []}
                      onChange={field.onChange}
                      loyaltyProgramId={loyaltyProgramId}
                      disabled={isPending || !!createdAccountId}
                    />
                  )}
                />

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
                  disabled={isPending || !!createdAccountId}
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
