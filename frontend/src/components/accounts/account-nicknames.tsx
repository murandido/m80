"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { createAccountNicknameAction, deleteAccountNicknameAction } from "@/actions/account"
import { AccountNickname } from "@/schemas/account"
import { NicknameTagsInput } from "@/components/people/nickname-tags-input"

interface AccountNicknamesProps {
  accountId: string
  loyaltyProgramId: string
  nicknames: AccountNickname[]
}

export function AccountNicknames({
  accountId,
  loyaltyProgramId,
  nicknames,
}: AccountNicknamesProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)

  function handleAdd(nickname: string) {
    setError(null)
    startTransition(async () => {
      const result = await createAccountNicknameAction(
        accountId,
        nickname,
        loyaltyProgramId,
      )
      if (result.error) {
        // e.g. 422 "Apelido já está em uso neste programa"
        setError(result.error)
        return
      }
      router.refresh()
    })
  }

  function handleRemove(nicknameId: string) {
    setError(null)
    startTransition(async () => {
      const result = await deleteAccountNicknameAction(accountId, nicknameId)
      if (result.error) {
        setError(result.error)
        return
      }
      router.refresh()
    })
  }

  return (
    <NicknameTagsInput
      tags={nicknames.map((n) => ({ key: n.id, label: n.nickname }))}
      onAdd={handleAdd}
      onRemove={handleRemove}
      disabled={isPending}
      error={error ?? undefined}
    />
  )
}
