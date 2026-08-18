"use client"

import type { Control, FieldValues, Path } from "react-hook-form"
import { Controller } from "react-hook-form"
import { FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

// How many people a program lets an account issue for, and how that is measured. The
// airlines change these without notice - Smiles has moved its cap more than once - so they
// are editable rather than only seeded, and they are the same three fields on create and on
// edit, hence one component.
//
// "Sem limite" is a real answer, not an empty form: it means nobody has looked this program's
// rule up yet, and the accounts screen then shows no counter rather than a counter at zero.

const NONE = "none"

const WINDOW_OPTIONS = [
  { value: "calendar_year", label: "Ano civil (zera em 1º de janeiro)" },
  { value: "rolling_12_months", label: "Últimos 12 meses" },
]

// Both modes cap the same thing - CPFs. The difference is reuse: on Smiles a CPF already
// used costs nothing to fly again, on LATAM it costs another slot.
const MODE_OPTIONS = [
  { value: "distinct_people", label: "CPF pode ser reaproveitado" },
  { value: "per_issuance", label: "CPF não pode ser reaproveitado" },
]

interface CPFLimitFieldsProps<T extends FieldValues> {
  control: Control<T>
  disabled: boolean
  // Prefixes the input ids, so the create form and every edit panel on the page keep
  // distinct labels.
  idPrefix: string
}

export function CPFLimitFields<T extends FieldValues>({
  control,
  disabled,
  idPrefix,
}: CPFLimitFieldsProps<T>) {
  return (
    <>
      <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
        Limite de CPFs
      </h3>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={"cpf_limit" as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}-cpf-limit`}>
                Quantas pessoas por período
              </FieldLabel>
              <Input
                {...field}
                value={field.value ?? ""}
                id={`${idPrefix}-cpf-limit`}
                inputMode="numeric"
                placeholder="Deixe vazio se o programa não tem limite conhecido"
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
          name={"cpf_limit_window" as Path<T>}
          control={control}
          render={({ field }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}-cpf-limit-window`}>Período</FieldLabel>
              <Select
                value={field.value || NONE}
                onValueChange={(value) => field.onChange(value === NONE ? "" : value)}
                disabled={disabled}
              >
                <SelectTrigger id={`${idPrefix}-cpf-limit-window`} className="bg-slate-200">
                  <SelectValue placeholder="Sem limite" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>Sem limite</SelectItem>
                  {WINDOW_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </>
          )}
        />
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={"cpf_limit_mode" as Path<T>}
          control={control}
          render={({ field }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}-cpf-limit-mode`}>Como conta</FieldLabel>
              <Select
                value={field.value || NONE}
                onValueChange={(value) => field.onChange(value === NONE ? "" : value)}
                disabled={disabled}
              >
                <SelectTrigger id={`${idPrefix}-cpf-limit-mode`} className="bg-slate-200">
                  <SelectValue placeholder="Sem limite" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>Sem limite</SelectItem>
                  {MODE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-slate-500">
                Na Smiles, emitir de novo para quem já voou não consome vaga nova. Na LATAM
                consome — o mesmo CPF ocupa uma vaga a cada emissão.
              </p>
            </>
          )}
        />
      </div>
    </>
  )
}
