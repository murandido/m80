"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Save } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { PersonCombobox } from "@/components/people/person-combobox"
import { setHolderAdministratorAction } from "@/actions/person"
import { PersonOption } from "@/schemas/person"

interface HolderAdministratorFieldProps {
  holderId: string
  people: PersonOption[]
  // The distinct administrators the holder's live accounts name today: one when they agree,
  // two or more when the row is flagged "Administradores diferentes", none when unset.
  currentIds: string[]
  // The administrator lives on the accounts, so a holder with none has nothing to set.
  hasAccounts: boolean
}

// One administrator for all of a holder's accounts (backend internal/account/administrators.go).
// Saving writes it on every live account, which is also how a conflict gets resolved.
export function HolderAdministratorField({
  holderId,
  people,
  currentIds,
  hasAccounts,
}: HolderAdministratorFieldProps) {
  const router = useRouter()
  const [value, setValue] = useState(
    currentIds.length === 1 ? currentIds[0] : "",
  )
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  const conflictNames = currentIds
    .map((id) => people.find((person) => person.id === id)?.name ?? "?")
    .join(", ")
  const unchanged =
    currentIds.length === 1 ? value === currentIds[0] : value === ""

  function handleSave() {
    setError(null)
    startTransition(async () => {
      const result = await setHolderAdministratorAction(holderId, value)
      if (result.error) {
        setError(result.error)
        return
      }
      router.refresh()
    })
  }

  if (!hasAccounts) {
    return (
      <p className="text-sm text-slate-400">
        Sem contas: o administrador é definido junto com a primeira conta.
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-1.5">
      {currentIds.length > 1 && (
        <p className="text-xs text-amber-800">
          As contas estão com administradores diferentes ({conflictNames}).
          Escolha um: ele vale para todas.
        </p>
      )}
      <div className="flex gap-2">
        <div className="flex-1">
          <PersonCombobox
            people={people.filter((person) => person.id !== holderId)}
            value={value}
            onChange={setValue}
            disabled={isPending}
            placeholder="Buscar administrador por nome ou apelido..."
          />
        </div>
        <Button
          type="button"
          onClick={handleSave}
          disabled={isPending || unchanged}
          className="bg-slate-800 hover:bg-slate-900"
          title="Salvar administrador"
        >
          {isPending ? <Spinner /> : <Save className="h-4 w-4" />}
          Salvar
        </Button>
      </div>
      {error && <p className="text-destructive text-xs">{error}</p>}
    </div>
  )
}
