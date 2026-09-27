"use client"

import { useState } from "react"
import { ChevronsUpDown } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

interface SaleNoteCellProps {
  note: string | null | undefined
  // null clears the note.
  onSave: (note: string | null) => void
  disabled?: boolean
}

const MAX_NOTE_LENGTH = 1000

// The sale's free-text line about its money. Edited in a popover rather than inline so a
// note longer than the cell still has room, and so a stray keystroke while scanning the
// table cannot rewrite one.
export function SaleNoteCell({ note, onSave, disabled }: SaleNoteCellProps) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState(note ?? "")

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) setText(note ?? "")
  }

  function save(value: string | null) {
    setOpen(false)
    const next = value?.trim() || null
    if (next !== (note || null)) onSave(next)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          title={note ?? undefined}
          className="h-9 w-full justify-between border-transparent bg-slate-200 font-normal hover:bg-slate-200"
        >
          <span className={cn("min-w-0 truncate", !note && "text-muted-foreground")}>
            {note || "Observação aqui..."}
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) min-w-64 p-2" align="start">
        <Textarea
          autoFocus
          value={text}
          maxLength={MAX_NOTE_LENGTH}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
              event.preventDefault()
              save(text)
            }
          }}
          placeholder="Ex: paga na sexta, cobrar junto com a volta"
          className="min-h-20 text-sm"
        />
        <div className="mt-2 flex justify-end gap-2">
          {note && (
            <Button type="button" variant="ghost" size="sm" onClick={() => save(null)}>
              Limpar
            </Button>
          )}
          <Button type="button" size="sm" onClick={() => save(text)}>
            Salvar
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
