"use client"

import { useEffect, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Eye, EyeOff, Hash, Save } from "lucide-react"
import { DateInput } from "@/components/ui/date-input"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { ACCOUNT_CATEGORY_OPTIONS } from "@/lib/account-category"
import {
  applyPaddedThousandsMask,
  formatPointsInThousands,
  parsePointsFromThousands,
  sanitizeThousandsInput,
} from "@/lib/points"
import { Account, updateAccountSchema } from "@/schemas/account"
import { PersonOption } from "@/schemas/person"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { updateAccountAction } from "@/actions/account"
import { PersonCombobox } from "@/components/people/person-combobox"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { AccountNicknames } from "./account-nicknames"

type FormData = z.infer<typeof updateAccountSchema.request>

interface EditAccountPanelProps {
  account: Account
  people: PersonOption[]
  loyaltyPrograms: LoyaltyProgram[]
  isOpen: boolean
  onSaved: () => void
}

function toFormValues(account: Account): FormData {
  return {
    loyalty_program_id: account.loyalty_program_id ?? "",
    login: account.login ?? "",
    password: account.password ?? "",
    holder_id: account.holder_id ?? "",
    administrator_id: account.administrator_id ?? "",
    category: account.category,
    waives_redemption_fee: account.waives_redemption_fee,
    owns_point_inventory: account.owns_point_inventory,
    default_profit_share_percent: account.default_profit_share_percent,
    real_balance_checked_on: account.real_balance_checked_on ?? "",
  }
}

// The stored balance as the field shows it: milheiros, the unit every point count in the
// panel is typed in.
function toBalanceText(account: Account): string {
  return account.real_balance_points === null || account.real_balance_points === undefined
    ? ""
    : formatPointsInThousands(account.real_balance_points)
}

export function EditAccountPanel({
  account,
  people,
  loyaltyPrograms,
  isOpen,
  onSaved,
}: EditAccountPanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  const [showPassword, setShowPassword] = useState(false)
  // Held outside the form because it is typed in milheiros and stored in points: letting
  // react-hook-form own the number would convert on every keystroke and eat the separator
  // halfway through "632,7".
  const [realBalance, setRealBalance] = useState(() => toBalanceText(account))

  const form = useForm<FormData>({
    resolver: zodResolver(updateAccountSchema.request),
    mode: "onTouched",
    defaultValues: toFormValues(account),
  })

  // Reset form whenever the panel closes, discarding unsaved edits.
  useEffect(() => {
    if (!isOpen) {
      form.reset(toFormValues(account))
      setRealBalance(toBalanceText(account))
    }
  }, [isOpen]) // eslint-disable-line react-hooks/exhaustive-deps

  const typedBalance = realBalance.trim() === "" ? null : parsePointsFromThousands(realBalance)
  const expected = account.expected_balance_points
  // The gap, when both numbers exist. Positive means the panel counts more points than the
  // airline shows - stock that was spent without the panel hearing about it.
  const divergence =
    expected !== null && expected !== undefined && typedBalance !== null
      ? expected - typedBalance
      : null

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateAccountAction(account.id, {
        ...data,
        // Only when it changed: an untouched field must not restamp the reading's date.
        ...(typedBalance !== null && typedBalance !== account.real_balance_points
          ? { real_balance_points: typedBalance }
          : {}),
      })
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
              Informações da conta
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Veja e/ou atualize as informações da conta aqui!
            </p>
          </div>

          <form onSubmit={form.handleSubmit(onSubmit)}>
            <Field className="flex flex-col gap-5">
              {/* UUID — read-only */}
              <div className="flex flex-col gap-1 text-sm">
                <FieldLabel htmlFor="edit-account-uuid">UUID</FieldLabel>
                <InputGroup className="bg-slate-100">
                  <InputGroupInput
                    id="edit-account-uuid"
                    type="text"
                    value={account.id}
                    readOnly
                    className="cursor-default select-all text-slate-400"
                  />
                  <InputGroupAddon align="inline-end">
                    <Hash className="h-4 w-4 text-slate-400" />
                  </InputGroupAddon>
                </InputGroup>
              </div>

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
                      <FieldLabel htmlFor="edit-account-loyalty-program-id">
                        Programa de fidelidade
                      </FieldLabel>
                      <LoyaltyProgramCombobox
                        loyaltyPrograms={loyaltyPrograms}
                        value={field.value ?? ""}
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
                      <FieldLabel htmlFor="edit-account-login">Login</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-account-login"
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

              {/* password */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="password"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-account-password">Senha</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-account-password"
                          type={showPassword ? "text" : "password"}
                          aria-invalid={fieldState.invalid}
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <InputGroupButton
                            type="button"
                            size="icon-xs"
                            onClick={() => setShowPassword((prev) => !prev)}
                            aria-label={showPassword ? "Ocultar senha" : "Mostrar senha"}
                          >
                            {showPassword ? (
                              <EyeOff className="h-4 w-4" />
                            ) : (
                              <Eye className="h-4 w-4" />
                            )}
                          </InputGroupButton>
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
                      <FieldLabel htmlFor="edit-account-category">Categoria</FieldLabel>
                      <Select
                        value={field.value ?? "unidentified"}
                        onValueChange={field.onChange}
                        disabled={isPending}
                      >
                        <SelectTrigger id="edit-account-category" className="bg-slate-200">
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
                        O controle de CPFs só vale em contas normais: a liminar suspende o
                        limite, e em conta não identificada não dá para afirmar que ele existe.
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
                        <FieldLabel htmlFor="edit-account-waives-redemption-fee">
                          Isenta de taxa de resgate
                        </FieldLabel>
                        <Switch
                          id="edit-account-waives-redemption-fee"
                          checked={field.value ?? false}
                          onCheckedChange={field.onChange}
                          disabled={isPending}
                        />
                      </div>
                      <p className="text-xs text-slate-500">
                        Contas Azul Diamante Unique não pagam a taxa de resgate. As emissões
                        feitas aqui passam a registrar quanto foi economizado; nas demais
                        contas a coluna fica vazia.
                      </p>
                    </>
                  )}
                />
              </div>


              {/* owns_point_inventory */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="owns_point_inventory"
                  control={form.control}
                  render={({ field }) => (
                    <>
                      <div className="flex items-center justify-between gap-3">
                        <FieldLabel htmlFor="edit-account-owns-point-inventory">
                          Estoque próprio de pontos
                        </FieldLabel>
                        <Switch
                          id="edit-account-owns-point-inventory"
                          checked={field.value ?? false}
                          onCheckedChange={field.onChange}
                          disabled={isPending}
                        />
                      </div>
                      <p className="text-xs text-slate-500">
                        Ligue quando os pontos desta conta foram comprados por nós. O custo passa a
                        vir dos lotes lançados em Estoque de pontos, e deixa de ser digitado em
                        Fornecedores.
                      </p>
                    </>
                  )}
                />
              </div>

              {/* default_profit_share_percent */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="default_profit_share_percent"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-account-profit-share">
                        Rateio padrão (%)
                      </FieldLabel>
                      <InputGroup>
                        <InputGroupInput
                          id="edit-account-profit-share"
                          type="text"
                          inputMode="numeric"
                          value={String(field.value ?? 0)}
                          onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
                            field.onChange(Number(event.target.value.replace(/\D/g, "")) || 0)
                          }
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">%</InputGroupAddon>
                      </InputGroup>
                      <p className="text-xs text-slate-500">
                        Quanto do lucro fica com a agência que emitiu, quando ela não é a que
                        bancou o lote. Cada lote congela esse valor ao ser criado, então mudar
                        aqui não mexe em nada que já foi acertado. Zero deixa o lucro inteiro com
                        quem bancou.
                      </p>
                      {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                    </>
                  )}
                />
              </div>

              {/* saldo previsto x saldo real */}
              <div className="flex flex-col gap-2 rounded-md border border-slate-200 p-3 text-sm">
                <div className="flex items-baseline justify-between gap-3">
                  <span className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                    Saldo previsto
                  </span>
                  {expected === null || expected === undefined ? (
                    <span className="text-xs text-slate-400">
                      só em conta com estoque próprio
                    </span>
                  ) : (
                    <span className="tabular-nums font-medium">
                      {applyPaddedThousandsMask(String(expected))} pontos
                    </span>
                  )}
                </div>
                {/* Derived from the batches and the bookings; the airline's own page is the
                    only place the real number exists, so somebody reads it and types it here. */}
                <p className="text-xs text-slate-500">
                  O previsto vem dos lotes menos o que foi emitido. O real é o que a companhia
                  mostra na página da conta — nenhum extrato traz saldo, então ele é conferido à
                  mão.
                </p>

                <div className="flex flex-wrap items-end gap-3">
                  <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <FieldLabel htmlFor="edit-account-real-balance">
                      Saldo real
                      <span className="ml-1 font-normal text-slate-400">em milheiros</span>
                    </FieldLabel>
                    <InputGroup>
                      <InputGroupInput
                        id="edit-account-real-balance"
                        inputMode="decimal"
                        placeholder="632,7"
                        value={realBalance}
                        onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
                          setRealBalance(sanitizeThousandsInput(event.target.value))
                        }
                        disabled={isPending}
                      />
                      <InputGroupAddon align="inline-end">mil</InputGroupAddon>
                    </InputGroup>
                    {typedBalance !== null && (
                      <p className="text-xs text-slate-500">
                        = {applyPaddedThousandsMask(String(typedBalance))} pontos
                      </p>
                    )}
                  </div>
                  <div className="flex flex-col gap-1">
                    <Controller
                      name="real_balance_checked_on"
                      control={form.control}
                      render={({ field }) => (
                        <>
                          <FieldLabel htmlFor="edit-account-balance-checked-on">
                            Conferido em
                          </FieldLabel>
                          <DateInput
                            id="edit-account-balance-checked-on"
                            value={field.value ?? ""}
                            onChange={field.onChange}
                            disabled={isPending}
                            className="w-44"
                          />
                        </>
                      )}
                    />
                  </div>
                </div>

                {divergence !== null &&
                  (divergence === 0 ? (
                    <p className="text-xs text-emerald-700">O previsto bate com o real.</p>
                  ) : (
                    <p className="text-xs text-amber-800">
                      {divergence > 0
                        ? `O painel conta ${applyPaddedThousandsMask(String(divergence))} pontos a mais do que a companhia — falta lançar alguma emissão.`
                        : `A companhia mostra ${applyPaddedThousandsMask(String(-divergence))} pontos a mais que o painel — falta lançar algum lote.`}
                    </p>
                  ))}
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
                      <FieldLabel htmlFor="edit-account-holder-id">Titular</FieldLabel>
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
                      <FieldLabel htmlFor="edit-account-administrator-id">
                        Administrador
                      </FieldLabel>
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
              <AccountNicknames
                accountId={account.id}
                loyaltyProgramId={account.loyalty_program_id}
                nicknames={account.nicknames}
              />

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
                  {isPending ? <Spinner /> : <Save className="h-4 w-4" />}
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
