"use client"

import { useEffect, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { MergePanel, PreviewLines } from "@/components/shared/merge-panel"
import { mergeAccountAction, previewMergeAccountAction } from "@/actions/account"
import {
  Account,
  AccountOption,
  LoyaltyProgramSummary,
  MergeAccountPreview,
} from "@/schemas/account"
import { PersonOption } from "@/schemas/person"
import { AccountCombobox } from "./account-combobox"

interface MergeAccountPanelProps {
  account: Account
  // Every account, unfiltered - the duplicate is usually the one the current search hid.
  accounts: AccountOption[]
  people: PersonOption[]
  loyaltyPrograms: LoyaltyProgramSummary[]
  title: string
  isOpen: boolean
  onMerged: () => void
}

function previewLines(preview: MergeAccountPreview): string[] {
  const lines: string[] = []
  if (preview.issuances > 0) {
    lines.push(
      preview.issuances === 1
        ? "1 emissão passa para esta conta"
        : `${preview.issuances} emissões passam para esta conta`,
    )
  }
  if (preview.nicknames > 0) {
    lines.push(
      preview.nicknames === 1 ? "1 apelido é movido" : `${preview.nicknames} apelidos são movidos`,
    )
  }
  if (preview.fields_to_fill?.length) {
    lines.push(`Campos preenchidos: ${preview.fields_to_fill.join(", ")}`)
  }
  if (preview.colliding_nicknames?.length) {
    lines.push(
      `Apelidos repetidos, que ficam só nesta conta: ${preview.colliding_nicknames.join(", ")}`,
    )
  }
  return lines
}

export function MergeAccountPanel({
  account,
  accounts,
  people,
  loyaltyPrograms,
  title,
  isOpen,
  onMerged,
}: MergeAccountPanelProps) {
  const router = useRouter()
  const [sourceId, setSourceId] = useState("")
  const [preview, setPreview] = useState<MergeAccountPreview | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  // Forget the pick whenever the panel closes - which a successful merge does too.
  // The panel stays mounted underneath the card, so without this reopening it showed
  // the account that was just merged away, with the old preview, ready to confirm against
  // a record that no longer exists. Adjusted during render rather than in an effect,
  // so the stale pick never paints.
  const [wasOpen, setWasOpen] = useState(isOpen)
  if (isOpen !== wasOpen) {
    setWasOpen(isOpen)
    if (!isOpen) {
      setSourceId("")
      setPreview(null)
      setError(null)
    }
  }

  useEffect(() => {
    if (!sourceId) {
      setPreview(null)
      return
    }

    let current = true
    previewMergeAccountAction(account.id, sourceId).then((result) => {
      if (!current) return
      if (result.error) {
        setError(result.error)
        setPreview(null)
        return
      }
      setError(null)
      setPreview(result.preview ?? null)
    })

    return () => {
      current = false
    }
  }, [account.id, sourceId])

  function handleConfirm() {
    if (!sourceId) return

    startTransition(async () => {
      const result = await mergeAccountAction(account.id, sourceId)
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
      keepLabel={title}
      subtitle="Escolha a conta duplicada. As emissões dela passam para esta, e ela é apagada."
      confirmLabel="Mesclar e apagar a conta escolhida"
      canConfirm={Boolean(sourceId)}
      isPending={isPending}
      error={error}
      onConfirm={handleConfirm}
    >
      {/* Same loyalty program only: the backend refuses the rest, since those issuances
          were flown on that program's miles. Filtering here keeps the refusal off screen. */}
      <AccountCombobox
        accounts={accounts.filter(
          (candidate) =>
            candidate.id !== account.id &&
            candidate.loyalty_program_id === account.loyalty_program_id,
        )}
        people={people}
        loyaltyPrograms={loyaltyPrograms}
        value={sourceId}
        onChange={setSourceId}
        placeholder="Buscar a conta duplicada..."
        disabled={isPending}
      />

      {preview && <PreviewLines lines={previewLines(preview)} />}
    </MergePanel>
  )
}
