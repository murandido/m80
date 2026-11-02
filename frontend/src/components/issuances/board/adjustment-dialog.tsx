"use client"

import { useEffect, useTransition } from "react"
import { Controller, useForm, useWatch } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { DateInput } from "@/components/ui/date-input"
import { todayISO } from "@/components/issuances/board/use-issuance-board"
import { cn } from "@/lib/utils"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import {
  applyPaddedThousandsMask,
  formatPointsInThousands,
  parsePointsFromThousands,
  sanitizeThousandsInput,
} from "@/lib/points"
import type { IssuanceAdjustment, IssuanceAdjustmentInput } from "@/schemas/issuance"

type ActionResult = { success: true } | { error: string }

// What the airline did: charged more, or gave some back. It is the sign of every amount,
// asked once instead of on each field, so an operator never types a minus.
type AdjustmentKind = "charge" | "refund"

const optionalPoints = z
  .string()
  .refine(
    (v) => v.trim() === "" || parsePointsFromThousands(v) !== null,
    "Informe os pontos em milheiros — 10 = 10.000",
  )
const optionalMoney = z
  .string()
  .refine((v) => v.trim() === "" || parseCurrencyBRLInput(v) !== null, "Valor inválido")

const adjustmentFormSchema = z
  .object({
    kind: z.enum(["charge", "refund"]),
    occurred_on: z.string().min(1, "Informe a data"),
    points: optionalPoints,
    taxes: optionalMoney,
    passed_to_client: z.boolean(),
    value_of_the_thousand: optionalMoney,
    taxes_passed_to_client: z.boolean(),
    reason: z.string().trim().min(1, "Descreva o que mudou na passagem"),
  })
  .refine(
    (values) =>
      Math.abs(parsePointsFromThousands(values.points) ?? 0) > 0 ||
      Math.abs(parseCurrencyBRLInput(values.taxes) ?? 0) > 0,
    { message: "Informe pontos ou taxas", path: ["points"] },
  )

type AdjustmentFormData = z.infer<typeof adjustmentFormSchema>

function formFor(
  adjustment: IssuanceAdjustment | null,
  issuanceRate: number | null,
): AdjustmentFormData {
  if (!adjustment) {
    return {
      kind: "charge",
      occurred_on: todayISO(),
      points: "",
      taxes: "",
      passed_to_client: true,
      value_of_the_thousand:
        issuanceRate !== null ? (issuanceRate / 100).toFixed(2).replace(".", ",") : "",
      taxes_passed_to_client: true,
      reason: "",
    }
  }
  const refund = adjustment.points < 0 || (adjustment.points === 0 && adjustment.taxes < 0)
  const passed = adjustment.value_of_the_thousand > 0 || adjustment.taxes_passed_to_client
  return {
    kind: refund ? "refund" : "charge",
    occurred_on: adjustment.occurred_on,
    points: adjustment.points !== 0 ? formatPointsInThousands(Math.abs(adjustment.points)) : "",
    taxes:
      adjustment.taxes !== 0
        ? (Math.abs(adjustment.taxes) / 100).toFixed(2).replace(".", ",")
        : "",
    passed_to_client: passed,
    value_of_the_thousand: passed
      ? (adjustment.value_of_the_thousand / 100).toFixed(2).replace(".", ",")
      : issuanceRate !== null
        ? (issuanceRate / 100).toFixed(2).replace(".", ",")
        : "",
    taxes_passed_to_client: adjustment.taxes_passed_to_client,
    reason: adjustment.reason,
  }
}

// The payload, signed. Absorbed means the client pays (or gets back) nothing: a zero rate
// and the taxes kept off the bill.
function toInput(values: AdjustmentFormData): IssuanceAdjustmentInput {
  const sign = values.kind === "refund" ? -1 : 1
  const points = Math.abs(parsePointsFromThousands(values.points) ?? 0)
  const taxes = Math.abs(parseCurrencyBRLInput(values.taxes) ?? 0)
  return {
    occurred_on: values.occurred_on,
    points: sign * points,
    taxes: sign * taxes,
    value_of_the_thousand: values.passed_to_client
      ? Math.abs(parseCurrencyBRLInput(values.value_of_the_thousand) ?? 0)
      : 0,
    taxes_passed_to_client: values.passed_to_client && values.taxes_passed_to_client,
    reason: values.reason.trim(),
  }
}

// A preview of what the server will work out (sale.AdjustmentClientCents): the points
// priced by magnitude and rounded up, the sign put back, the taxes added when passed on.
// The figure saved is always the server's.
export function previewClientCents(input: IssuanceAdjustmentInput): number {
  const magnitude = Math.ceil((Math.abs(input.points) * input.value_of_the_thousand) / 1000)
  const pointsCents = input.points < 0 ? -magnitude : magnitude
  return pointsCents + (input.taxes_passed_to_client ? input.taxes : 0)
}

interface AdjustmentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The alteration being corrected, or null for a new one.
  adjustment: IssuanceAdjustment | null
  // The issuance's own rate, offered as the rate the client pays for the extra miles.
  issuanceRate: number | null
  onSave: (input: IssuanceAdjustmentInput) => Promise<ActionResult>
}

export function AdjustmentDialog({
  open,
  onOpenChange,
  adjustment,
  issuanceRate,
  onSave,
}: AdjustmentDialogProps) {
  const [isPending, startTransition] = useTransition()
  const form = useForm<AdjustmentFormData>({
    resolver: zodResolver(adjustmentFormSchema),
    mode: "onTouched",
    defaultValues: formFor(adjustment, issuanceRate),
  })

  // Re-seeded on every open: the same dialog serves a new alteration and the correction of
  // any of the listed ones, and a tab left open overnight must not offer yesterday.
  useEffect(() => {
    if (open) form.reset(formFor(adjustment, issuanceRate))
  }, [open, adjustment, issuanceRate, form])

  // useWatch rather than form.watch(): the preview and the labels follow every keystroke,
  // and watch() opts the whole component out of the React Compiler.
  const values = useWatch({ control: form.control }) as AdjustmentFormData
  const refund = values.kind === "refund"
  const typedPoints = parsePointsFromThousands(values.points)
  const preview = previewClientCents(toInput(values))

  function onSubmit(submitted: AdjustmentFormData) {
    startTransition(async () => {
      const result = await onSave(toInput(submitted))
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success(adjustment ? "Alteração corrigida." : "Alteração registrada.")
      onOpenChange(false)
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {/* Keys typed here must not reach the panel's own Escape handling or the board's
            shortcuts, which still see them through the React tree. */}
        <form onSubmit={form.handleSubmit(onSubmit)} onKeyDown={(e) => e.stopPropagation()}>
          <DialogHeader>
            <DialogTitle>{adjustment ? "Corrigir alteração" : "Registrar alteração"}</DialogTitle>
            <DialogDescription>
              O que a companhia cobrou a mais ou devolveu depois da emissão. Os valores
              originais da emissão não mudam.
            </DialogDescription>
          </DialogHeader>

          <div className="my-4 flex flex-col gap-4 text-sm">
            <Controller
              name="kind"
              control={form.control}
              render={({ field }) => (
                <div role="radiogroup" aria-label="Tipo de alteração" className="grid grid-cols-2 gap-2">
                  {(
                    [
                      ["charge", "Cobrança extra"],
                      ["refund", "Reembolso"],
                    ] as const
                  ).map(([kind, label]) => (
                    <button
                      key={kind}
                      type="button"
                      role="radio"
                      aria-checked={field.value === kind}
                      disabled={isPending}
                      onClick={() => field.onChange(kind satisfies AdjustmentKind)}
                      className={cn(
                        "rounded-md border px-3 py-2 font-medium transition-colors",
                        field.value === kind
                          ? kind === "charge"
                            ? "border-amber-400 bg-amber-50 text-amber-900"
                            : "border-emerald-400 bg-emerald-50 text-emerald-900"
                          : "border-slate-200 text-slate-500 hover:bg-slate-50",
                      )}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              )}
            />

            <Controller
              name="occurred_on"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="adjustment_occurred_on">Data da alteração</FieldLabel>
                  <DateInput
                    id="adjustment_occurred_on"
                    value={field.value}
                    onChange={field.onChange}
                    aria-invalid={fieldState.invalid}
                    disabled={isPending}
                  />
                  {fieldState.invalid ? (
                    <FieldError errors={[fieldState.error]} />
                  ) : (
                    <p className="text-xs text-slate-500">
                      O dia em que a companhia {refund ? "devolveu" : "cobrou"}. É por ele que a
                      alteração entra no custeio e no acerto do mês.
                    </p>
                  )}
                </Field>
              )}
            />

            <div className="grid gap-4 sm:grid-cols-2">
              <Controller
                name="points"
                control={form.control}
                render={({ field, fieldState }) => (
                  <Field>
                    <FieldLabel htmlFor="adjustment_points">
                      {refund ? "Pontos devolvidos" : "Pontos cobrados"}
                    </FieldLabel>
                    <InputGroup>
                      <InputGroupInput
                        {...field}
                        id="adjustment_points"
                        onChange={(e) => field.onChange(sanitizeThousandsInput(e.target.value))}
                        inputMode="decimal"
                        title="Em milheiros: 10 = 10.000, 10,5 = 10.500"
                        placeholder="10,5"
                        aria-invalid={fieldState.invalid}
                        disabled={isPending}
                      />
                      <InputGroupAddon align="inline-end">
                        <InputGroupText>mil</InputGroupText>
                      </InputGroupAddon>
                    </InputGroup>
                    {fieldState.invalid ? (
                      <FieldError errors={[fieldState.error]} />
                    ) : (
                      typedPoints !== null && (
                        <p className="text-xs text-slate-500">
                          = {applyPaddedThousandsMask(String(typedPoints))} pontos
                        </p>
                      )
                    )}
                  </Field>
                )}
              />

              <Controller
                name="taxes"
                control={form.control}
                render={({ field, fieldState }) => (
                  <Field>
                    <FieldLabel htmlFor="adjustment_taxes">
                      {refund ? "Taxas devolvidas" : "Taxas pagas"}
                    </FieldLabel>
                    <InputGroup>
                      <InputGroupAddon align="inline-start">
                        <InputGroupText>R$</InputGroupText>
                      </InputGroupAddon>
                      <InputGroupInput
                        {...field}
                        id="adjustment_taxes"
                        inputMode="decimal"
                        placeholder="0,00"
                        aria-invalid={fieldState.invalid}
                        disabled={isPending}
                      />
                    </InputGroup>
                    {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                  </Field>
                )}
              />
            </div>

            <Controller
              name="passed_to_client"
              control={form.control}
              render={({ field }) => (
                <Field>
                  <div className="flex items-center justify-between gap-3">
                    <FieldLabel htmlFor="adjustment_passed_to_client">
                      {refund ? "Devolver ao cliente" : "Cobrar do cliente"}
                    </FieldLabel>
                    <Switch
                      id="adjustment_passed_to_client"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isPending}
                    />
                  </div>
                  <p className="text-xs text-slate-500">
                    {refund
                      ? "Desligado, a agência fica com o reembolso e o lucro do mês sobe."
                      : "Desligado, a agência absorve o custo e o valor da venda não muda."}
                  </p>
                </Field>
              )}
            />

            {values.passed_to_client && (
              <div className="grid gap-4 sm:grid-cols-2">
                <Controller
                  name="value_of_the_thousand"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <Field>
                      <FieldLabel htmlFor="adjustment_rate">Milheiro do cliente</FieldLabel>
                      <InputGroup>
                        <InputGroupAddon align="inline-start">
                          <InputGroupText>R$</InputGroupText>
                        </InputGroupAddon>
                        <InputGroupInput
                          {...field}
                          id="adjustment_rate"
                          inputMode="decimal"
                          placeholder="0,00"
                          aria-invalid={fieldState.invalid}
                          disabled={isPending}
                        />
                      </InputGroup>
                      {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                    </Field>
                  )}
                />
                <Controller
                  name="taxes_passed_to_client"
                  control={form.control}
                  render={({ field }) => (
                    <Field>
                      <div className="flex items-center justify-between gap-3 sm:mt-7">
                        <FieldLabel htmlFor="adjustment_taxes_passed">
                          {refund ? "Devolver as taxas" : "Cobrar as taxas"}
                        </FieldLabel>
                        <Switch
                          id="adjustment_taxes_passed"
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          disabled={isPending}
                        />
                      </div>
                    </Field>
                  )}
                />
              </div>
            )}

            <Controller
              name="reason"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="adjustment_reason">Motivo</FieldLabel>
                  <Textarea
                    {...field}
                    id="adjustment_reason"
                    rows={2}
                    placeholder="Ex.: GRU→CNF trocado para GRU→BHZ, voo de 20/11"
                    aria-invalid={fieldState.invalid}
                    disabled={isPending}
                  />
                  {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                </Field>
              )}
            />

            <p className="rounded-md bg-slate-100 px-3 py-2 text-slate-700">
              {preview === 0
                ? "A venda não muda."
                : preview > 0
                  ? `A venda aumenta ${formatCurrencyBRL(preview)}.`
                  : `O cliente recebe ${formatCurrencyBRL(-preview)} de volta.`}
            </p>
          </div>

          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={isPending}>
                Voltar
              </Button>
            </DialogClose>
            <Button type="submit" disabled={isPending}>
              {isPending && <Spinner />}
              {isPending ? "Salvando..." : adjustment ? "Salvar correção" : "Registrar"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
