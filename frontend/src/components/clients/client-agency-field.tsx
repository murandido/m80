"use client"

import { Controller, type Control, type FieldValues, type Path } from "react-hook-form"
import { FieldError, FieldLabel } from "@/components/ui/field"
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import type { Agency } from "@/schemas/agency"

// The agency a client buys through. Shared by the create form and the edit panel. The list
// comes from /agencies, already narrowed to the caller's own, so nobody can pick an agency
// they cannot see - and a client only ever gets linked to sales of this same agency.
export function ClientAgencyField<T extends FieldValues>({
  control,
  name,
  agencies,
  disabled,
}: {
  control: Control<T>
  name: Path<T>
  agencies: Agency[]
  disabled?: boolean
}) {
  return (
    <Controller
      name={name}
      control={control}
      render={({ field, fieldState }) => (
        <>
          <FieldLabel>
            Agência <span className="text-destructive">*</span>
          </FieldLabel>
          <AgencyCombobox
            agencies={agencies}
            value={field.value ?? ""}
            onChange={field.onChange}
            placeholder="Selecione a agência"
            disabled={disabled}
            clearable={false}
          />
          {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
        </>
      )}
    />
  )
}
