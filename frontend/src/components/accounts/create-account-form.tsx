"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { KeyRound, Plus, Save, User, UserPlus, X } from "lucide-react"
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
import { createPersonSchema, PersonOption } from "@/schemas/person"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { createAccountAction, createHolderWithAccountAction } from "@/actions/account"
import { PersonCombobox } from "@/components/people/person-combobox"
import { PersonFields } from "@/components/people/person-fields"
import { PersonNicknamesField } from "@/components/people/person-nicknames-field"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"

const accountFieldsSchema = createAccountSchema.request.omit({ holder_id: true })

// Two shapes of the same form. Registering a holder, the program is optional - leaving it
// blank creates only the person (an administrator who holds no account). Adding a program to
// a holder who already exists, the holder part is not on screen and the program is the point.
const newHolderSchema = z.object({
  holder: createPersonSchema.request,
  account: accountFieldsSchema.extend({
    loyalty_program_id: z.union([z.literal(""), z.string().uuid("UUID inválido")]),
  }),
})

const addProgramSchema = z.object({
  holder: z.any(),
  account: accountFieldsSchema,
})

type FormData = z.infer<typeof newHolderSchema>

interface CreateAccountFormProps {
  people: PersonOption[]
  loyaltyPrograms: LoyaltyProgram[]
  // Set when the form lives inside a holder's row: it then adds a program to that holder
  // instead of registering a new one.
  holderId?: string
}

const emptyHolder: FormData["holder"] = {
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
  nicknames: [],
}

export function CreateAccountForm({ people, loyaltyPrograms, holderId }: CreateAccountFormProps) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()
  // Guards against creating the records twice: only set when they were already created but
  // some nicknames failed (rare race), keeping the panel open so the user can see the
  // warning instead of silently losing it.
  const [createdId, setCreatedId] = useState<string | null>(null)
  const isAddingProgram = !!holderId
  const idPrefix = holderId ? `add-${holderId}-` : "new-"

  const form = useForm<FormData>({
    resolver: zodResolver(isAddingProgram ? addProgramSchema : newHolderSchema),
    mode: "onTouched",
    defaultValues: {
      holder: emptyHolder,
      account: {
        loyalty_program_id: "",
        login: "",
        password: "",
        administrator_id: "",
        category: "unidentified" as const,
        waives_redemption_fee: false,
      },
    },
  })

  function handleToggle() {
    if (isOpen) {
      form.reset()
      setCreatedId(null)
    }
    setIsOpen((prev) => !prev)
  }

  function onSubmit(data: FormData) {
    if (createdId) return

    startTransition(async () => {
      const result = holderId
        ? await createAccountAction({ ...data.account, holder_id: holderId })
        : await createHolderWithAccountAction(
            data.holder,
            data.account.loyalty_program_id ? data.account : null,
          )

      if ("error" in result) {
        form.setError("root", { message: result.error })
        return
      }

      router.refresh()

      if (result.failedNicknames?.length) {
        setCreatedId("accountId" in result ? result.accountId : result.personId)
        form.setError("root", {
          message: `Cadastro feito. Apelido(s) não adicionado(s) por já estar(em) em uso: ${result.failedNicknames.join(", ")}`,
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
      {isAddingProgram ? (
        <Button
          type="button"
          variant="outline"
          onClick={handleToggle}
          className="border-dashed border-slate-400 text-slate-700 hover:bg-white"
        >
          {isOpen ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
          {isOpen ? "Cancelar" : "Adicionar programa"}
        </Button>
      ) : (
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
      )}

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
                {!isAddingProgram && (
                  <>
                    <PersonFields
                      control={form.control}
                      namePrefix="holder."
                      idPrefix={idPrefix}
                      disabled={isPending}
                    />

                    <Separator />
                    <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                      Apelidos do titular
                    </h3>
                    <Controller
                      name="holder.nicknames"
                      control={form.control}
                      render={({ field }) => (
                        <PersonNicknamesField
                          value={field.value ?? []}
                          onChange={field.onChange}
                          disabled={isPending || !!createdId}
                        />
                      )}
                    />

                    <Separator />
                  </>
                )}

                <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                  Conta de programa
                </h3>
                {!isAddingProgram && (
                  <p className="-mt-3 text-xs text-slate-500">
                    Deixe o programa em branco para cadastrar só o titular - um administrador que
                    não tem conta própria, por exemplo.
                  </p>
                )}

                {/* loyalty_program_id */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="account.loyalty_program_id"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor={`${idPrefix}loyalty_program_id`}>
                          Programa de fidelidade{" "}
                          {isAddingProgram && <span className="text-destructive">*</span>}
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
                    name="account.login"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor={`${idPrefix}login`}>Login</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id={`${idPrefix}login`}
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
                    name="account.password"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor={`${idPrefix}password`}>Senha</FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id={`${idPrefix}password`}
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
                    name="account.category"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor={`${idPrefix}category`}>Categoria</FieldLabel>
                        <Select
                          value={field.value ?? "unidentified"}
                          onValueChange={field.onChange}
                          disabled={isPending}
                        >
                          <SelectTrigger id={`${idPrefix}category`} className="bg-slate-200">
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
                    name="account.waives_redemption_fee"
                    control={form.control}
                    render={({ field }) => (
                      <>
                        <div className="flex items-center justify-between gap-3">
                          <FieldLabel htmlFor={`${idPrefix}waives-redemption-fee`}>
                            Isenta de taxa de resgate
                          </FieldLabel>
                          <Switch
                            id={`${idPrefix}waives-redemption-fee`}
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
                  Administrador
                </h3>

                {/* administrator_id */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="account.administrator_id"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor={`${idPrefix}administrator_id`}>Administrador</FieldLabel>
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
                  disabled={isPending || !!createdId}
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
