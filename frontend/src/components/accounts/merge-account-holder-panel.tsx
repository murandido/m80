"use client"

import { useEffect, useMemo, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { MergePanel, PreviewLines } from "@/components/shared/merge-panel"
import { mergeRowAction, previewMergeRowAction } from "@/actions/person"
import { AccountOption } from "@/schemas/account"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import {
  MergeRowPreview,
  MergeRowSource,
  Person,
  PersonOption,
} from "@/schemas/person"
import { PersonCombobox } from "@/components/people/person-combobox"

interface MergeAccountHolderPanelProps {
  holder: Person
  // Every holder, unfiltered: the duplicate the operator is after is often exactly the one
  // the current search hid, since the two are registered under different names.
  people: PersonOption[]
  // The accounts nobody holds, which are rows of their own and can be absorbed too.
  orphanAccounts: AccountOption[]
  loyaltyPrograms: LoyaltyProgram[]
  isOpen: boolean
  onMerged: () => void
}

// Orphan accounts are offered in the same picker as people, so they are dressed as one.
// The prefix keeps the two id spaces apart and tells handleConfirm which kind was picked.
const ORPHAN_PREFIX = "account:"

function toSource(pickedId: string): MergeRowSource | null {
  if (!pickedId) return null
  return pickedId.startsWith(ORPHAN_PREFIX)
    ? { kind: "account", id: pickedId.slice(ORPHAN_PREFIX.length) }
    : { kind: "person", id: pickedId }
}

function plural(count: number, one: string, many: string): string {
  return count === 1 ? `1 ${one}` : `${count} ${many}`
}

function previewLines(preview: MergeRowPreview): string[] {
  const lines: string[] = []
  for (const pair of preview.accounts_merged ?? []) {
    const which = pair.drop_login ? ` (${pair.drop_login})` : ""
    let line = `${pair.program_name}${which} é mesclada na conta ${pair.program_name} deste titular`
    if (pair.issuances > 0) {
      line += ` - ${plural(pair.issuances, "emissão é movida", "emissões são movidas")}`
    }
    lines.push(line)
    if (pair.colliding_nicknames?.length) {
      lines.push(
        `Apelidos repetidos arquivados: ${pair.colliding_nicknames.join(", ")}`,
      )
    }
  }
  if (preview.accounts_moved > 0) {
    lines.push(
      `${plural(preview.accounts_moved, "conta passa", "contas passam")} a ser deste titular`,
    )
  }
  if (preview.administered > 0) {
    lines.push(
      `${plural(preview.administered, "conta administrada passa", "contas administradas passam")} a ser administrada(s) por este titular`,
    )
  }
  if (preview.nicknames > 0) {
    lines.push(
      plural(preview.nicknames, "apelido é movido", "apelidos são movidos"),
    )
  }
  if (preview.fields_to_fill?.length) {
    lines.push(`Campos preenchidos: ${preview.fields_to_fill.join(", ")}`)
  }
  return lines
}

export function MergeAccountHolderPanel({
  holder,
  people,
  orphanAccounts,
  loyaltyPrograms,
  isOpen,
  onMerged,
}: MergeAccountHolderPanelProps) {
  const router = useRouter()
  const [pickedId, setPickedId] = useState("")
  const [preview, setPreview] = useState<MergeRowPreview | null>(null)
  const [keepNickname, setKeepNickname] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  const candidates = useMemo<PersonOption[]>(() => {
    const programName = new Map(
      loyaltyPrograms.map((program) => [program.id, program.name]),
    )
    const orphans = orphanAccounts.map((account) => {
      const id = `${ORPHAN_PREFIX}${account.id}`
      const label =
        account.login ?? account.nicknames[0]?.nickname ?? "Conta sem login"
      return {
        id,
        name: `${label} (${programName.get(account.loyalty_program_id) ?? "programa"}, sem titular${account.deleted_at ? ", arquivada" : ""})`,
        nicknames: account.nicknames.map((nickname) => ({
          id: nickname.id,
          person_id: id,
          nickname: nickname.nickname,
          created_at: "",
        })),
      }
    })
    return [
      ...people.filter((candidate) => candidate.id !== holder.id),
      ...orphans,
    ]
  }, [people, orphanAccounts, loyaltyPrograms, holder.id])

  const picked = candidates.find((candidate) => candidate.id === pickedId)

  // Forget the pick whenever the panel closes - which a successful merge does too. Adjusted
  // during render rather than in an effect, so the stale pick never paints.
  const [wasOpen, setWasOpen] = useState(isOpen)
  if (isOpen !== wasOpen) {
    setWasOpen(isOpen)
    if (!isOpen) {
      setPickedId("")
      setPreview(null)
      setError(null)
      setKeepNickname(true)
    }
  }

  useEffect(() => {
    const source = toSource(pickedId)
    if (!source) return

    let current = true
    previewMergeRowAction(holder.id, source).then((result) => {
      if (!current) return
      if (result.error) {
        setError(result.error)
        setPreview(null)
        return
      }
      setError(null)
      setPreview(result.preview ?? null)
    })

    // The operator can change their mind mid-request; a stale preview describing the
    // previous pick would be the worst possible thing to confirm against.
    return () => {
      current = false
    }
  }, [holder.id, pickedId])

  function handlePick(id: string) {
    setPickedId(id)
    setPreview(null)
  }

  function handleConfirm() {
    const source = toSource(pickedId)
    if (!source) return

    startTransition(async () => {
      const result = await mergeRowAction(holder.id, source, keepNickname)
      if (result.error) {
        setError(result.error)
        return
      }
      onMerged()
      router.refresh()
    })
  }

  return (
    <MergePanel
      isOpen={isOpen}
      keepLabel={holder.name}
      subtitle="Escolha a linha duplicada. Ela é absorvida por esta: contas de programa repetidas viram uma só, e o cadastro duplicado é apagado."
      confirmLabel={picked ? `Mesclar e apagar “${picked.name}”` : "Mesclar"}
      canConfirm={Boolean(preview)}
      isPending={isPending}
      error={error}
      onConfirm={handleConfirm}
    >
      <PersonCombobox
        people={candidates}
        value={pickedId}
        onChange={handlePick}
        placeholder="Buscar a linha duplicada..."
        disabled={isPending}
      />

      {preview && (
        <>
          <PreviewLines lines={previewLines(preview)} />

          {preview.nickname_from_name && (
            <label className="flex items-start gap-2 text-xs text-slate-600">
              <input
                type="checkbox"
                checked={keepNickname}
                onChange={(event) => setKeepNickname(event.target.checked)}
                disabled={isPending}
                className="mt-0.5"
              />
              {/* Without this the duplicate comes back: the bot creates a person for any
                  name it does not recognise, and that name is precisely what it just met. */}
              <span>
                Guardar “{preview.nickname_from_name}” como apelido de “
                {holder.name}”, para o bot não cadastrar essa pessoa de novo
              </span>
            </label>
          )}
        </>
      )}
    </MergePanel>
  )
}
