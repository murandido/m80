"use client"

import { Button } from "@/components/ui/button"
import { FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

const kindOptions = [
  { value: false, label: "Equipe" },
  { value: true, label: "Parceiro" },
] as const

// Team or partner, shared by the create form and the edit panel. A partner issues on their
// own behalf, so the payments control fills the client of every sale they issue with the
// name below - their own name, in capitals, when it is left empty.
export function PartnerField({
  isPartner,
  onPartnerChange,
  clientName,
  onClientNameChange,
  userName,
  disabled,
}: {
  isPartner: boolean
  onPartnerChange: (isPartner: boolean) => void
  clientName: string
  onClientNameChange: (name: string) => void
  // The fallback shown as the placeholder: what the API uses when the name is left empty.
  userName: string
  disabled?: boolean
}) {
  return (
    <>
      <FieldLabel>Vínculo</FieldLabel>
      <div className="flex gap-2">
        {kindOptions.map((option) => (
          <Button
            key={option.label}
            type="button"
            size="sm"
            onClick={() => onPartnerChange(option.value)}
            disabled={disabled}
            className={cn(
              "min-w-24 flex-1 transition-colors duration-150",
              isPartner === option.value
                ? "bg-slate-800 text-white hover:bg-slate-900"
                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
            )}
          >
            {option.label}
          </Button>
        ))}
      </div>
      {isPartner && (
        <div className="mt-2 flex flex-col gap-1">
          <FieldLabel htmlFor="partner-client-name">Cliente nos pagamentos</FieldLabel>
          <Input
            id="partner-client-name"
            value={clientName}
            onChange={(event) => onClientNameChange(event.target.value)}
            placeholder={userName.trim().toUpperCase() || "NOME DO CLIENTE"}
            disabled={disabled}
            className="bg-slate-200 uppercase"
            maxLength={120}
          />
          <FieldDescription>
            As emissões deste usuário entram em Controle de pagamentos já com este cliente.
          </FieldDescription>
        </div>
      )}
    </>
  )
}
