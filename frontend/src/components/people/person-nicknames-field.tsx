"use client"

import { useState, useTransition } from "react"
import { checkNicknameAvailabilityAction } from "@/actions/person"
import { NicknameTagsInput } from "./nickname-tags-input"

interface PersonNicknamesFieldProps {
  value: string[]
  onChange: (value: string[]) => void
  disabled?: boolean
}

export function PersonNicknamesField({
  value,
  onChange,
  disabled,
}: PersonNicknamesFieldProps) {
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)

  function handleAdd(nickname: string) {
    setError(null)
    startTransition(async () => {
      const result = await checkNicknameAvailabilityAction(nickname)

      if ("error" in result) {
        setError(result.error)
        return
      }
      if (!result.available) {
        setError("Apelido já está em uso")
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
