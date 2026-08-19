"use client"

import type { Control, FieldValues, Path } from "react-hook-form"
import { Controller } from "react-hook-form"
import { FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

// What the airline charges to issue an award ticket close to the flight, and how close is
// close. Azul's "taxa de resgate" is the one this was built for: R$ 69,90 domestic per
// passenger and per trecho inside 120 days, and nothing at all beyond that.
//
// Editable rather than only seeded, for the same reason the CPF cap is: the published rule
// moves. Azul launched this at 90 days with a different price per tier and had flattened it
// within a year. Changing a value here does not rewrite fees already computed - those are
// frozen on the issuance - so it only affects what is issued or re-synced from now on.
//
// Empty is a real answer: it means this program charges no redemption fee we know of, and
// nothing is computed at all. That is not the same as a fee of zero.

interface RedemptionFeeFieldsProps<T extends FieldValues> {
  control: Control<T>
  disabled: boolean
  // Prefixes the input ids, so the create form and every edit panel on the page keep
  // distinct labels.
  idPrefix: string
}

export function RedemptionFeeFields<T extends FieldValues>({
  control,
  disabled,
  idPrefix,
}: RedemptionFeeFieldsProps<T>) {
  return (
    <>
      <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
        Taxa de resgate
      </h3>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={"redemption_fee_domestic" as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}-redemption-fee-domestic`}>
                Voo nacional (R$ por passageiro, por trecho)
              </FieldLabel>
              <Input
                {...field}
                value={field.value ?? ""}
                id={`${idPrefix}-redemption-fee-domestic`}
                inputMode="decimal"
                placeholder="Deixe vazio se o programa não cobra taxa de resgate"
                className="bg-slate-200"
                disabled={disabled}
              />
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={"redemption_fee_international" as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}-redemption-fee-international`}>
                Voo internacional (R$ por passageiro, por trecho)
              </FieldLabel>
              <Input
                {...field}
                value={field.value ?? ""}
                id={`${idPrefix}-redemption-fee-international`}
                inputMode="decimal"
                placeholder="Deixe vazio se o programa não cobra taxa de resgate"
                className="bg-slate-200"
                disabled={disabled}
              />
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={"redemption_fee_advance_days" as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}-redemption-fee-advance-days`}>
                Antecedência que isenta (dias)
              </FieldLabel>
              <Input
                {...field}
                value={field.value ?? ""}
                id={`${idPrefix}-redemption-fee-advance-days`}
                inputMode="numeric"
                placeholder="120"
                className="bg-slate-200"
                disabled={disabled}
              />
              <p className="text-xs text-slate-500">
                Emissões para voos com essa antecedência ou mais não pagam taxa. O valor só
                é registrado nas contas marcadas como isentas, onde ele é a economia.
              </p>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>
    </>
  )
}
