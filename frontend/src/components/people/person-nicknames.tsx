"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { createPersonNicknameAction, deletePersonNicknameAction } from "@/actions/person"
import { PersonNickname } from "@/schemas/person"
import { NicknameTagsInput } from "./nickname-tags-input"

interface PersonNicknamesProps {
  personId: string
  nicknames: PersonNickname[]
}

export function PersonNicknames({ personId, nicknames }: PersonNicknamesProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)

  function handleAdd(nickname: string) {
    setError(null)
    startTransition(async () => {
      const result = await createPersonNicknameAction(personId, nickname)
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
      const result = await deletePersonNicknameAction(personId, nicknameId)
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
