"use client"

import { useState, useTransition } from "react"
import { checkAgencyNicknameAvailabilityAction } from "@/actions/agency"
import { NicknameTagsInput } from "@/components/people/nickname-tags-input"

interface AgencyNicknamesFieldProps {
  value: string[]
  onChange: (value: string[]) => void
  disabled?: boolean
}

export function AgencyNicknamesField({
  value,
  onChange,
  disabled,
}: AgencyNicknamesFieldProps) {
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)

  function handleAdd(nickname: string) {
    setError(null)
    startTransition(async () => {
      const result = await checkAgencyNicknameAvailabilityAction(nickname)

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
