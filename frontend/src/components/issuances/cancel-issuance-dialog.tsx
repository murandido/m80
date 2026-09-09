"use client"

import { useState, useTransition } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { toast } from "sonner"
import { Ban } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
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
import { useGridContext } from "@/components/issuances/grid-context"
import { parseCurrencyBRLInput } from "@/lib/currency"
import {
  applyPaddedThousandsMask,
  parsePointsFromThousands,
  sanitizeThousandsInput,
} from "@/lib/points"
import type { CancelIssuanceInput } from "@/schemas/issuance"

// The form carries display strings (money uses a comma decimal), converted to the numeric
// cents/points payload on submit. All fields are optional - a bare cancel is valid.
const cancelFormSchema = z.object({
  // Points are typed in milheiros here for the same reason they are in the grid's Pontos cell:
  // it is the unit operators quote, and a refund is usually read straight off the issuance
  // they are cancelling. Anything else would make the two screens disagree about what "10" means.
  refunded_points: z
    .string()
    .refine(
      (v) => v.trim() === "" || parsePointsFromThousands(v) !== null,
      "Informe os pontos em milheiros — 10 = 10.000",
    ),
  refunded_taxes: z
    .string()
    .refine((v) => v.trim() === "" || parseCurrencyBRLInput(v) !== null, "Valor inválido"),
  cancellation_fee: z
    .string()
    .refine((v) => v.trim() === "" || parseCurrencyBRLInput(v) !== null, "Valor inválido"),
  // The booking was made outside the panel and was only registered here so this estorno
  // has something to hang off. It is a property of the cancellation, not of the refund,
  // so it carries no amount.
  cancellation_only: z.boolean(),
  // The day the airline gave the points back. It is what dates the refund on the costing's
  // timeline and in the supplier balance, so a cancellation recorded a week late still
  // belongs to the week it happened.
  cancelled_on: z.string().min(1, "Informe a data do cancelamento"),
})

type CancelFormData = z.infer<typeof cancelFormSchema>

// Today in the operator's own calendar day, which is what the backend resolves the
// stored timestamp against.
function todayISO(): string {
  const now = new Date()
  return new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10)
}

const emptyForm: CancelFormData = {
  refunded_points: "",
  refunded_taxes: "",
  cancellation_fee: "",
  cancellation_only: false,
  cancelled_on: todayISO(),
}

export function CancelIssuanceButton({ r }: { r: number }) {
  const { requestSaveActiveCell, onCancelRow } = useGridContext()
  const [open, setOpen] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<CancelFormData>({
    resolver: zodResolver(cancelFormSchema),
    mode: "onTouched",
    defaultValues: emptyForm,
  })

  // Watched rather than read off the Controller's field: the confirmation line has to update on
  // every keystroke, and this is the only place in the form that reads a value it does not own.
  const refundedPoints = parsePointsFromThousands(form.watch("refunded_points"))

  function onSubmit(values: CancelFormData) {
    startTransition(async () => {
      const data: CancelIssuanceInput = {}
      const points = parsePointsFromThousands(values.refunded_points)
      if (points !== null) data.refunded_points = points
      const taxes = parseCurrencyBRLInput(values.refunded_taxes)
      if (taxes !== null) data.refunded_taxes = taxes
      const fee = parseCurrencyBRLInput(values.cancellation_fee)
      if (fee !== null) data.cancellation_fee = fee
      if (values.cancellation_only) data.cancellation_only = true
      // Always sent, today included: the backend would default to now() anyway, and
      // sending it makes the day the operator saw the one that gets written.
      data.cancelled_on = values.cancelled_on

      const result = await onCancelRow(r, data)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Emissão cancelada.")
      form.reset({ ...emptyForm, cancelled_on: todayISO() })
      setOpen(false)
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        // Re-read today rather than reusing emptyForm's: a tab left open overnight would
        // otherwise offer yesterday.
        if (!next) form.reset({ ...emptyForm, cancelled_on: todayISO() })
        setOpen(next)
      }}
    >
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          title="Cancelar emissão"
          className="h-6 w-6 opacity-0 transition-opacity group-hover/row:opacity-100"
          // Persist the cell currently being edited before opening, mirroring the delete
          // button - otherwise an in-flight edit could be lost.
          onClick={() => requestSaveActiveCell()}
        >
          <Ban className="h-3.5 w-3.5 text-amber-600" />
        </Button>
      </DialogTrigger>
      <DialogContent>
        {/* Radix portals this to document.body, but React synthetic events still bubble
            through the React tree - where this dialog is a descendant of the grid's
            keyboard handler. Without this, typing in the form also drives the spreadsheet
            (e.g. seeds an edit in the selected cell). Stop keydown here to isolate it. */}
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          onKeyDown={(e) => e.stopPropagation()}
        >
          <DialogHeader>
            <DialogTitle>Cancelar emissão</DialogTitle>
            <DialogDescription>
              Informe os valores de reembolso, se houver. Todos os campos são opcionais — deixe
              em branco para cancelar sem reembolso.
            </DialogDescription>
          </DialogHeader>

          <div className="my-4 flex flex-col gap-4 text-sm">
            <Controller
              name="refunded_points"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="refunded_points">Pontos reembolsados</FieldLabel>
                  <InputGroup>
                    <InputGroupInput
                      {...field}
                      id="refunded_points"
                      // Same mask as the grid's cell: digits and a single comma, no thousands
                      // grouping. A pasted "10.000" therefore loses its dot - which is why the
                      // line below spells out the count this will actually refund.
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
                    refundedPoints !== null && (
                      <p className="text-xs text-slate-500">
                        = {applyPaddedThousandsMask(String(refundedPoints))} pontos
                      </p>
                    )
                  )}
                </Field>
              )}
            />

            <Controller
              name="refunded_taxes"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="refunded_taxes">Taxas reembolsadas</FieldLabel>
                  <InputGroup>
                    <InputGroupAddon align="inline-start">
                      <InputGroupText>R$</InputGroupText>
                    </InputGroupAddon>
                    <InputGroupInput
                      {...field}
                      id="refunded_taxes"
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
              name="cancellation_only"
              control={form.control}
              render={({ field }) => (
                <Field>
                  <div className="flex items-center justify-between gap-3">
                    <FieldLabel htmlFor="cancellation_only">
                      A emissão não existia no sistema
                    </FieldLabel>
                    <Switch
                      id="cancellation_only"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isPending}
                    />
                  </div>
                  <p className="text-xs text-slate-500">
                    Ligue quando a reserva foi feita fora do painel e você a cadastrou só
                    para lançar este estorno. Os pontos deixam de contar como emitidos e o
                    saldo do fornecedor fica só com o reembolso — sem precisar dobrar os
                    pontos reembolsados.
                  </p>
                </Field>
              )}
            />

            <Controller
              name="cancelled_on"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="cancelled_on">Data do cancelamento</FieldLabel>
                  <InputGroup>
                    <InputGroupInput
                      {...field}
                      id="cancelled_on"
                      type="date"
                      aria-invalid={fieldState.invalid}
                      disabled={isPending}
                    />
                  </InputGroup>
                  {fieldState.invalid ? (
                    <FieldError errors={[fieldState.error]} />
                  ) : (
                    <p className="text-xs text-slate-500">
                      O dia em que a companhia devolveu os pontos. É por ele que o estorno entra
                      no custeio e no acerto — mude se o cancelamento não foi hoje.
                    </p>
                  )}
                </Field>
              )}
            />

            <Controller
              name="cancellation_fee"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="cancellation_fee">Multa de cancelamento</FieldLabel>
                  <InputGroup>
                    <InputGroupAddon align="inline-start">
                      <InputGroupText>R$</InputGroupText>
                    </InputGroupAddon>
                    <InputGroupInput
                      {...field}
                      id="cancellation_fee"
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

          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={isPending}>
                Voltar
              </Button>
            </DialogClose>
            <Button type="submit" variant="destructive" disabled={isPending}>
              {isPending && <Spinner />}
              {isPending ? "Cancelando..." : "Cancelar emissão"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
