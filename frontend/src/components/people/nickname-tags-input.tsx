"use client"

import { useState, type KeyboardEvent } from "react"
import { Tag as TagIcon, X } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"

export interface NicknameTag {
  key: string
  label: string
}

interface NicknameTagsInputProps {
  tags: NicknameTag[]
  onAdd: (value: string) => void
  onRemove: (key: string) => void
  disabled?: boolean
  error?: string
  placeholder?: string
}

export function NicknameTagsInput({
  tags,
  onAdd,
  onRemove,
  disabled,
  error,
  placeholder = "Digite um apelido e aperte Enter",
}: NicknameTagsInputProps) {
  const [inputValue, setInputValue] = useState("")

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter") return
    // This field lives inside a <form> — without this, Enter would submit
    // the whole form instead of just adding the tag.
    event.preventDefault()

    const value = inputValue.trim()
    if (!value) return

    const alreadyAdded = tags.some(
      (tag) => tag.label.toLowerCase() === value.toLowerCase(),
    )
    setInputValue("")
    if (alreadyAdded) return

    onAdd(value)
  }

  return (
    <div className="flex flex-col gap-2 text-sm">
      <InputGroup className="bg-slate-200">
        <InputGroupInput
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          disabled={disabled}
          aria-invalid={!!error}
        />
        <InputGroupAddon align="inline-end">
          <TagIcon className="text-foreground h-4 w-4" />
        </InputGroupAddon>
      </InputGroup>
      {error && <p className="text-destructive text-sm">{error}</p>}
      {tags.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {tags.map((tag) => (
            <Badge key={tag.key} variant="secondary" className="gap-1 py-1 pr-1 pl-2.5">
              {tag.label}
              <button
                type="button"
                onClick={() => onRemove(tag.key)}
                disabled={disabled}
                className="rounded-full p-0.5 hover:bg-slate-300"
                aria-label={`Remover ${tag.label}`}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
    </div>
  )
}
