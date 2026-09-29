"use client"

import { Button } from "@/components/ui/button"
import { FieldLabel } from "@/components/ui/field"
import { cn } from "@/lib/utils"
import type { User } from "@/schemas/user"

type Role = User["role"]

// The order the buttons appear in, least privileged first. "dev" is only ever the value
// sent to the API - the panel always shows the Portuguese label.
const roleOptions: { value: Role; label: string }[] = [
  { value: "user", label: "Usuário" },
  { value: "admin", label: "Admin" },
  { value: "dev", label: "Desenvolvedor" },
]

// Shared by the create form and the edit panel, which carried a byte-for-byte copy of this
// markup each. Extracted when the third role arrived rather than pasting it a third and
// fourth time.
export function RoleSelector({
  value,
  onChange,
  disabled,
  allowDev = false,
}: {
  value: Role
  onChange: (role: Role) => void
  disabled?: boolean
  // Only a dev hands out the dev role - the API refuses it from anyone else - so the
  // option is not offered to an admin at all.
  allowDev?: boolean
}) {
  const options = allowDev ? roleOptions : roleOptions.filter((option) => option.value !== "dev")

  return (
    <>
      <FieldLabel>
        Perfil <span className="text-destructive">*</span>
      </FieldLabel>
      {/* flex-wrap because "Desenvolvedor" is far longer than the other two: on the narrow
          edit panel the third button drops to its own line instead of squeezing the label. */}
      <div className="flex flex-wrap gap-2">
        {options.map((option) => (
          <Button
            key={option.value}
            type="button"
            size="sm"
            onClick={() => onChange(option.value)}
            disabled={disabled}
            className={cn(
              "min-w-24 flex-1 transition-colors duration-150",
              value === option.value
                ? "bg-slate-800 text-white hover:bg-slate-900"
                : "border border-slate-300 bg-white text-slate-600 hover:bg-slate-100",
            )}
          >
            {option.label}
          </Button>
        ))}
      </div>
    </>
  )
}
