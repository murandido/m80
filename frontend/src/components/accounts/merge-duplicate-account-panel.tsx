"use client"

import { useEffect, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { MergePanel, PreviewLines } from "@/components/shared/merge-panel"
import {
  mergeAccountAction,
  previewMergeAccountAction,
} from "@/actions/account"
import { Account, MergeAccountPreview } from "@/schemas/account"

interface MergeDuplicateAccountPanelProps {
  // The account being folded away.
  account: Account
  // The holder's other live accounts in the same program - one of them survives.
  targets: Account[]
  isOpen: boolean
  onMerged: () => void
}

function accountLabel(account: Account): string {
  return `${account.loyalty_program.name}${account.login ? ` (${account.login})` : " (sem login)"}`
}

function previewLines(
  preview: MergeAccountPreview,
  account: Account,
): string[] {
  const lines: string[] = []
  if (preview.issuances > 0) {
    lines.push(
      preview.issuances === 1
        ? "1 emissão é movida para a conta que fica"
        : `${preview.issuances} emissões são movidas para a conta que fica`,
    )
  }
  if (preview.nicknames > 0) {
    lines.push(
      preview.nicknames === 1
        ? "1 apelido é movido"
        : `${preview.nicknames} apelidos são movidos`,
    )
  }
  if (preview.colliding_nicknames?.length) {
    lines.push(
      `Apelidos repetidos arquivados: ${preview.colliding_nicknames.join(", ")}`,
    )
  }
  if (account.login) {
    lines.push(`O login ${account.login} deixa de existir no painel`)
  }
  return lines
}

// A holder has one live account per program (backend internal/account/programs.go); a row
// that still shows two is fixed by folding one into the other. The account in this block is
// the one that goes: its issuances and aliases move to the one the operator keeps, and it is
// deleted for real.
export function MergeDuplicateAccountPanel({
  account,
  targets,
  isOpen,
  onMerged,
}: MergeDuplicateAccountPanelProps) {
  const router = useRouter()
  const [keepId, setKeepId] = useState(targets[0]?.id ?? "")
  const [preview, setPreview] = useState<MergeAccountPreview | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  const keep = targets.find((target) => target.id === keepId)

  useEffect(() => {
    if (!isOpen || !keepId) return

    let current = true
    previewMergeAccountAction(keepId, account.id).then((result) => {
      if (!current) return
      if (result.error) {
        setError(result.error)
        setPreview(null)
        return
      }
      setError(null)
      setPreview(result.preview ?? null)
    })
    // A stale preview for the previous pick is the worst thing to confirm against.
    return () => {
      current = false
    }
  }, [isOpen, keepId, account.id])

  function handleConfirm() {
    if (!keepId) return
    startTransition(async () => {
      const result = await mergeAccountAction(keepId, account.id)
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
      keepLabel={keep ? accountLabel(keep) : account.loyalty_program.name}
      subtitle={`Este titular tem mais de uma conta na ${account.loyalty_program.name}. Esta é mesclada na escolhida e apagada.`}
      confirmLabel={`Mesclar e apagar ${accountLabel(account)}`}
      canConfirm={Boolean(preview)}
      isPending={isPending}
      error={error}
      onConfirm={handleConfirm}
    >
      {targets.length > 1 && (
        <div className="flex flex-col gap-1 text-sm">
          <span className="text-xs text-slate-500">Conta que fica</span>
          {targets.map((target) => (
            <label key={target.id} className="flex items-center gap-2">
              <input
                type="radio"
                name={`keep-${account.id}`}
                checked={keepId === target.id}
                onChange={() => {
                  setKeepId(target.id)
                  setPreview(null)
                }}
                disabled={isPending}
              />
              {accountLabel(target)}
            </label>
          ))}
        </div>
      )}

      {preview && <PreviewLines lines={previewLines(preview, account)} />}
    </MergePanel>
  )
}
