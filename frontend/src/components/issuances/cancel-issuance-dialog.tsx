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
import type { CancelIssuanceInput } from "@/schemas/issuance"

// The form carries display strings (money uses a comma decimal), converted to the numeric
// cents/points payload on submit. All fields are optional - a bare cancel is valid.
const cancelFormSchema = z.object({
  refunded_points: z
    .string()
    .refine((v) => v.trim() === "" || /^\d+$/.test(v.trim()), "Informe um número inteiro de pontos"),
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
})

type CancelFormData = z.infer<typeof cancelFormSchema>

const emptyForm: CancelFormData = {
  refunded_points: "",
  refunded_taxes: "",
  cancellation_fee: "",
  cancellation_only: false,
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

  function onSubmit(values: CancelFormData) {
    startTransition(async () => {
      const data: CancelIssuanceInput = {}
      const points = values.refunded_points.trim()
      if (points !== "") data.refunded_points = parseInt(points, 10)
      const taxes = parseCurrencyBRLInput(values.refunded_taxes)
      if (taxes !== null) data.refunded_taxes = taxes
      const fee = parseCurrencyBRLInput(values.cancellation_fee)
      if (fee !== null) data.cancellation_fee = fee
      if (values.cancellation_only) data.cancellation_only = true

      const result = await onCancelRow(r, data)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Emissão cancelada.")
      form.reset(emptyForm)
      setOpen(false)
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) form.reset(emptyForm)
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
                      inputMode="numeric"
                      placeholder="0"
                      aria-invalid={fieldState.invalid}
                      disabled={isPending}
                    />
                  </InputGroup>
                  {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
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
