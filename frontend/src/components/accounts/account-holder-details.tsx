"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Menu, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { Person } from "@/schemas/person"
import { EditPersonPanel } from "@/components/people/edit-person-panel"
import { DeleteOverlay } from "@/components/people/delete-overlay"
import { PersonNicknames } from "@/components/people/person-nicknames"

interface AccountHolderDetailsProps {
  holder: Person
  // A holder can only be deleted once nothing points at them - accounts.holder_id is
  // ON DELETE RESTRICT - so the button only exists on a row with no accounts left.
  canDelete: boolean
}

// "1990-05-17T00:00:00Z" → "17/05/1990". Sliced rather than parsed: the date carries no
// time of day, and a Date would shift it by the browser's offset.
function formatBirthDate(value: string | null): string | null {
  if (!value) return null
  const [year, month, day] = value.slice(0, 10).split("-")
  return `${day}/${month}/${year}`
}

function formatAddress(holder: Person): string | null {
  const street = [holder.address, holder.address_number]
    .filter(Boolean)
    .join(", ")
  const parts = [
    street,
    holder.complement,
    holder.neighborhood,
    [holder.city, holder.state].filter(Boolean).join("/"),
    holder.zip_code && `CEP ${holder.zip_code}`,
  ].filter(Boolean)
  return parts.length > 0 ? parts.join(" - ") : null
}

// The holder's own data, under their accounts: who the accounts belong to, read-only until
// the operator opens the edit panel.
export function AccountHolderDetails({
  holder,
  canDelete,
}: AccountHolderDetailsProps) {
  const router = useRouter()
  const [isEditOpen, setIsEditOpen] = useState(false)
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  const [isDeleted, setIsDeleted] = useState(false)

  const rows: [string, string | null][] = [
    ["Nome", holder.name],
    ["CPF", holder.cpf],
    ["Nascimento", formatBirthDate(holder.birth_date)],
    ["E-mail", holder.email],
    ["Telefone", holder.phone],
    ["Endereço", formatAddress(holder)],
  ]

  return (
    <div className="flex flex-col">
      <div
        className={cn(
          "relative overflow-hidden border border-slate-200 bg-white transition-opacity duration-300",
          isEditOpen ? "rounded-t-md" : "rounded-md",
          isDeleted ? "opacity-0" : "opacity-100",
        )}
        onTransitionEnd={() => isDeleted && router.refresh()}
      >
        <div className="flex items-start justify-between gap-3 px-4 py-3">
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
            {rows.map(([label, value]) => (
              <div key={label} className="contents">
                <dt className="text-slate-500">{label}</dt>
                <dd className="text-slate-800">
                  {value ?? <span className="text-slate-400">—</span>}
                </dd>
              </div>
            ))}
          </dl>
          <div className="flex shrink-0 gap-2">
            {canDelete && (
              <Button
                variant="outline"
                size="icon"
                title="Excluir titular"
                aria-label="Excluir titular"
                onClick={() => setIsDeleteOpen(true)}
                className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            <Button
              variant="outline"
              size="icon"
              title="Editar titular"
              aria-label="Editar titular"
              onClick={() => setIsEditOpen((prev) => !prev)}
              className={cn(
                "transition-colors duration-200",
                isEditOpen
                  ? "border-cyan-600 bg-cyan-600 text-white hover:border-cyan-700 hover:bg-cyan-700"
                  : "border-slate-800 text-slate-800 hover:border-cyan-400 hover:bg-cyan-400 hover:text-white active:border-cyan-800 active:bg-cyan-800 active:text-white",
              )}
            >
              <Menu className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {/* One list for the holder and every program they have: an alias added here is
            also how the bot finds each of their accounts (internal/account/nicknames.go). */}
        <div className="flex flex-col gap-1.5 border-t border-slate-100 px-4 py-3">
          <span className="text-xs text-slate-500">Apelidos</span>
          <PersonNicknames personId={holder.id} nicknames={holder.nicknames} />
        </div>

        {canDelete && (
          <DeleteOverlay
            personName={holder.name}
            personId={holder.id}
            isOpen={isDeleteOpen}
            onDismissed={() => setIsDeleteOpen(false)}
            onDeleted={() => setIsDeleted(true)}
          />
        )}
      </div>

      <EditPersonPanel
        person={holder}
        isOpen={isEditOpen}
        onSaved={() => setIsEditOpen(false)}
      />
    </div>
  )
}
