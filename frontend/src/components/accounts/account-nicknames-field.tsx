"use client"

import { useState, useTransition } from "react"
import { checkAccountNicknameAvailabilityAction } from "@/actions/account"
import { NicknameTagsInput } from "@/components/people/nickname-tags-input"

interface AccountNicknamesFieldProps {
  value: string[]
  onChange: (value: string[]) => void
  loyaltyProgramId: string
  disabled?: boolean
}

export function AccountNicknamesField({
  value,
  onChange,
  loyaltyProgramId,
  disabled,
}: AccountNicknamesFieldProps) {
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)

  function handleAdd(nickname: string) {
    if (!loyaltyProgramId) {
      setError("Informe o programa de fidelidade antes de adicionar apelidos")
      return
    }

    setError(null)
    startTransition(async () => {
      const result = await checkAccountNicknameAvailabilityAction(
        nickname,
        loyaltyProgramId,
      )

      if ("error" in result) {
        setError(result.error)
        return
      }
      if (!result.available) {
        setError("Apelido já está em uso neste programa")
        return
      }

      onChange([...value, nickname])
    })
  }

  function handleRemove(tagValue: string) {
    onChange(value.filter((v) => v !== tagValue))
  }

  return (
    <NicknameTagsInput
      tags={value.map((v) => ({ key: v, label: v }))}
      onAdd={handleAdd}
      onRemove={handleRemove}
      disabled={disabled || isPending}
      error={error ?? undefined}
    />
  )
}
