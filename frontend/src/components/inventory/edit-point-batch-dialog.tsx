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
import { DateInput } from "@/components/ui/date-input"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import {
  formatPointsInThousands,
  formatSignedPaddedPoints,
  parsePointsFromThousands,
  sanitizeThousandsInput,
} from "@/lib/points"
import { updatePointBatchAction, type PointBatchInput } from "@/actions/inventory"
import type { Agency } from "@/schemas/agency"
import type { PointBatch, PointBatchOrigin } from "@/schemas/inventory"

const batchFormSchema = z.object({
  origin_id: z.string(),
  acquired_on: z.string().min(1, "Informe a data"),
  points: z.string().refine((v) => {
    const points = parsePointsFromThousands(v)
    return points !== null && points > 0
  }, "Informe os pontos em milheiros — 632,7 = 632.700"),
  // Blank is zero, the same as on the launch form: a club subscription costs nothing.
  cost: z
    .string()
    .refine(
      (v) => v.trim() === "" || (parseCurrencyBRLInput(v) ?? -1) >= 0,
      "Custo inválido",
    ),
  funding_agency_id: z.string().min(1, "Informe a agência que bancou o lote"),
  profit_share_percent: z
    .string()
    .refine((v) => /^\d{1,3}$/.test(v.trim()) && Number(v) <= 100, "Rateio entre 0 e 100"),
  note: z.string(),
})

type BatchFormData = z.infer<typeof batchFormSchema>

function formFor(batch: PointBatch | null): BatchFormData {
  return {
    origin_id: batch?.origin_id ?? "",
    acquired_on: batch?.acquired_on ?? "",
    points: batch ? formatPointsInThousands(batch.points) : "",
    cost: batch ? (batch.cost_cents / 100).toFixed(2).replace(".", ",") : "",
    funding_agency_id: batch?.funding_agency_id ?? "",
    profit_share_percent: batch ? String(batch.profit_share_percent) : "",
    note: batch?.note ?? "",
  }
}

// Only what the operator actually changed goes to the API. A batch corrected for its cost
// must not have its date or agency rewritten from a form that merely displayed them.
function changedFields(
  batch: PointBatch,
  values: BatchFormData,
): Partial<Omit<PointBatchInput, "accountId" | "opening">> {
  const changes: Partial<Omit<PointBatchInput, "accountId" | "opening">> = {}
  if (!batch.opening && values.origin_id && values.origin_id !== batch.origin_id) {
    changes.originId = values.origin_id
  }
  if (values.acquired_on !== batch.acquired_on) changes.acquiredOn = values.acquired_on
  const points = parsePointsFromThousands(values.points)
  if (points !== null && points !== batch.points) changes.points = points
  const cost = values.cost.trim() === "" ? 0 : parseCurrencyBRLInput(values.cost)
  if (cost !== null && cost !== batch.cost_cents) changes.costCents = cost
  if (values.funding_agency_id !== batch.funding_agency_id) {
    changes.fundingAgencyId = values.funding_agency_id
  }
  const share = Number(values.profit_share_percent)
  if (share !== batch.profit_share_percent) changes.profitSharePercent = share
  if (values.note.trim() !== (batch.note ?? "")) changes.note = values.note.trim()
  return changes
}

interface EditPointBatchDialogProps {
  // The batch being corrected; null keeps the dialog closed.
  batch: PointBatch | null
  onClose: () => void
  origins: PointBatchOrigin[]
  agencies: Agency[]
}

export function EditPointBatchDialog({
  batch,
  onClose,
  origins,
  agencies,
}: EditPointBatchDialogProps) {
  const [isPending, startTransition] = useTransition()
  const form = useForm<BatchFormData>({
    resolver: zodResolver(batchFormSchema),
    mode: "onTouched",
    defaultValues: formFor(batch),
  })

  // Re-seeded whenever another batch is opened: one dialog serves every row of the table.
  useEffect(() => {
    if (batch) form.reset(formFor(batch))
  }, [batch, form])

  // useWatch rather than form.watch(): watch() opts the component out of the React Compiler.
  const values = useWatch({ control: form.control }) as BatchFormData
  const typedPoints = parsePointsFromThousands(values.points ?? "")
  const typedCost =
    (values.cost ?? "").trim() === "" ? null : parseCurrencyBRLInput(values.cost ?? "")

  function onSubmit(submitted: BatchFormData) {
    if (!batch) return
    if (!batch.opening && !submitted.origin_id) {
      form.setError("origin_id", { message: "Informe a origem do lote" })
      return
    }
    const changes = changedFields(batch, submitted)
    if (Object.keys(changes).length === 0) {
      onClose()
      return
    }
    startTransition(async () => {
      const result = await updatePointBatchAction(batch.id, changes)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Lote atualizado. O custeio é refeito no próximo cálculo.")
      onClose()
    })
  }

  return (
    <Dialog open={batch !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form onSubmit={form.handleSubmit(onSubmit)}>
          <DialogHeader>
            <DialogTitle>Editar lote</DialogTitle>
            <DialogDescription>
              A conta e o saldo de partida não mudam aqui: para isso, exclua o lote e lance de
              novo.
            </DialogDescription>
          </DialogHeader>

          <div className="my-4 grid gap-4 text-sm sm:grid-cols-2">
            <Controller
              name="origin_id"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="batch_edit_origin">Origem</FieldLabel>
                  <Select
                    value={field.value}
                    onValueChange={field.onChange}
                    disabled={isPending || batch?.opening}
                  >
                    <SelectTrigger id="batch_edit_origin" aria-invalid={fieldState.invalid}>
                      <SelectValue placeholder={batch?.opening ? "Saldo de partida" : "Escolha"} />
                    </SelectTrigger>
                    <SelectContent>
                      {origins.map((origin) => (
                        <SelectItem key={origin.id} value={origin.id}>
                          {origin.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                </Field>
              )}
            />

            <Controller
              name="acquired_on"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="batch_edit_date">Data</FieldLabel>
                  <DateInput
                    id="batch_edit_date"
                    value={field.value}
                    onChange={field.onChange}
                    aria-invalid={fieldState.invalid}
                    disabled={isPending}
                  />
                  {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                </Field>
              )}
            />

            <Controller
              name="points"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="batch_edit_points">Qnt. de Pontos</FieldLabel>
                  <InputGroup>
                    <InputGroupInput
                      {...field}
                      id="batch_edit_points"
                      onChange={(e) => field.onChange(sanitizeThousandsInput(e.target.value))}
                      inputMode="decimal"
                      placeholder="632,7"
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
                        = {formatSignedPaddedPoints(typedPoints)} pontos
                      </p>
                    )
                  )}
                </Field>
              )}
            />

            <Controller
              name="cost"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="batch_edit_cost">Custo</FieldLabel>
                  <InputGroup>
                    <InputGroupAddon align="inline-start">
                      <InputGroupText>R$</InputGroupText>
                    </InputGroupAddon>
                    <InputGroupInput
                      {...field}
                      id="batch_edit_cost"
                      inputMode="decimal"
                      placeholder="8.941,72"
                      aria-invalid={fieldState.invalid}
                      disabled={isPending}
                    />
                  </InputGroup>
                  {/* A dot and a comma are both the decimal separator, so say what the
                      field resolves to, as the launch form does. */}
                  {fieldState.invalid ? (
                    <FieldError errors={[fieldState.error]} />
                  ) : (
                    typedCost !== null && (
                      <p className="text-xs text-slate-500">= {formatCurrencyBRL(typedCost)}</p>
                    )
                  )}
                </Field>
              )}
            />

            <Controller
              name="funding_agency_id"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="batch_edit_agency">Bancado por</FieldLabel>
                  <Select value={field.value} onValueChange={field.onChange} disabled={isPending}>
                    <SelectTrigger id="batch_edit_agency" aria-invalid={fieldState.invalid}>
                      <SelectValue placeholder="Escolha" />
                    </SelectTrigger>
                    <SelectContent>
                      {agencies.map((agency) => (
                        <SelectItem key={agency.id} value={agency.id}>
                          {agency.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                </Field>
              )}
            />

            <Controller
              name="profit_share_percent"
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel htmlFor="batch_edit_share">Rateio</FieldLabel>
                  <InputGroup>
                    <InputGroupInput
                      {...field}
                      id="batch_edit_share"
                      inputMode="numeric"
                      placeholder="50"
                      aria-invalid={fieldState.invalid}
                      disabled={isPending}
                    />
                    <InputGroupAddon align="inline-end">
                      <InputGroupText>%</InputGroupText>
                    </InputGroupAddon>
                  </InputGroup>
                  {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                </Field>
              )}
            />

            <Controller
              name="note"
              control={form.control}
              render={({ field }) => (
                <Field className="sm:col-span-2">
                  <FieldLabel htmlFor="batch_edit_note">Observação</FieldLabel>
                  <Textarea
                    {...field}
                    id="batch_edit_note"
                    rows={2}
                    placeholder="Ex.: CUSTO PENDENTE"
                    disabled={isPending}
                  />
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
            <Button type="submit" disabled={isPending}>
              {isPending && <Spinner />}
              {isPending ? "Salvando..." : "Salvar"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
