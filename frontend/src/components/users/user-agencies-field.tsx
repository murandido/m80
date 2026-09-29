"use client"

import { FieldLabel } from "@/components/ui/field"
import { AgencyMultiCombobox } from "@/components/agencies/agency-multi-combobox"
import type { Agency } from "@/schemas/agency"

// The agencies a user may see and act on. Shared by the create form and the edit panel, and
// only ever rendered for a dev editing a non-dev account: a dev's role already covers every
// agency, and nobody else may change the grants.
export function UserAgenciesField({
  agencies,
  value,
  onChange,
  disabled,
}: {
  agencies: Agency[]
  value: string[]
  onChange: (ids: string[]) => void
  disabled?: boolean
}) {
  return (
    <>
      <FieldLabel>Agências</FieldLabel>
      <AgencyMultiCombobox
        agencies={agencies}
        value={value}
        onChange={onChange}
        disabled={disabled}
        emptyLabel="Nenhuma agência"
      />
      <p className="text-xs text-slate-500">
        O usuário só vê emissões, vendas e clientes destas agências.
      </p>
    </>
  )
}
