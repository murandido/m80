"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { createAgencyNicknameAction, deleteAgencyNicknameAction } from "@/actions/agency"
import { AgencyNickname } from "@/schemas/agency"
import { NicknameTagsInput } from "@/components/people/nickname-tags-input"

interface AgencyNicknamesProps {
  agencyId: string
  nicknames: AgencyNickname[]
}

export function AgencyNicknames({ agencyId, nicknames }: AgencyNicknamesProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)

  function handleAdd(nickname: string) {
    setError(null)
    startTransition(async () => {
      const result = await createAgencyNicknameAction(agencyId, nickname)
      if (result.error) {
        // e.g. 422 "Apelido já está em uso"
        setError(result.error)
        return
      }
      router.refresh()
    })
  }

  function handleRemove(nicknameId: string) {
    setError(null)
    startTransition(async () => {
      const result = await deleteAgencyNicknameAction(agencyId, nicknameId)
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
