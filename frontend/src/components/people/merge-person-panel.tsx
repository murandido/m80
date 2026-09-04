"use client"

import { useEffect, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { MergePanel, PreviewLines } from "@/components/shared/merge-panel"
import { mergePersonAction, previewMergePersonAction } from "@/actions/person"
import { MergePersonPreview, Person, PersonOption } from "@/schemas/person"
import { PersonCombobox } from "./person-combobox"

interface MergePersonPanelProps {
  person: Person
  // Every owner, unfiltered: the duplicate the operator is after is often exactly the one
  // the current search hid, since the two are registered under different names.
  people: PersonOption[]
  isOpen: boolean
  onMerged: () => void
}

function previewLines(preview: MergePersonPreview): string[] {
  const lines: string[] = []
  if (preview.accounts > 0) {
    lines.push(
      preview.accounts === 1
        ? "1 conta passa a ter este proprietário"
        : `${preview.accounts} contas passam a ter este proprietário`,
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
  return lines
}

export function MergePersonPanel({
  person,
  people,
  isOpen,
  onMerged,
}: MergePersonPanelProps) {
  const router = useRouter()
  const [sourceId, setSourceId] = useState("")
  const [preview, setPreview] = useState<MergePersonPreview | null>(null)
  const [keepNickname, setKeepNickname] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  const source = people.find((candidate) => candidate.id === sourceId)

  useEffect(() => {
    if (!sourceId) {
      setPreview(null)
      return
    }

    let current = true
    previewMergePersonAction(person.id, sourceId).then((result) => {
      if (!current) return
      if (result.error) {
        setError(result.error)
        setPreview(null)
        return
      }
      setError(null)
      setPreview(result.preview ?? null)
    })

    // The operator can change their mind mid-request; a stale preview describing the
    // previous pick would be the worst possible thing to confirm against.
    return () => {
      current = false
    }
  }, [person.id, sourceId])

  function handleConfirm() {
    if (!sourceId) return

    startTransition(async () => {
      const result = await mergePersonAction(person.id, sourceId, keepNickname)
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
      keepLabel={person.name}
      subtitle="Escolha o cadastro duplicado. Ele é absorvido por este e apagado."
      confirmLabel={source ? `Mesclar e apagar “${source.name}”` : "Mesclar"}
      canConfirm={Boolean(sourceId)}
      isPending={isPending}
      error={error}
      onConfirm={handleConfirm}
    >
      <PersonCombobox
        people={people.filter((candidate) => candidate.id !== person.id)}
        value={sourceId}
        onChange={setSourceId}
        placeholder="Buscar o cadastro duplicado..."
        disabled={isPending}
      />

      {preview && (
        <>
          <PreviewLines lines={previewLines(preview)} />

          {preview.nickname_from_name && (
            <label className="flex items-start gap-2 text-xs text-slate-600">
              <input
                type="checkbox"
                checked={keepNickname}
                onChange={(event) => setKeepNickname(event.target.checked)}
                disabled={isPending}
                className="mt-0.5"
              />
              {/* Without this the duplicate comes back: the bot creates a person for any
                  name it does not recognise, and that name is precisely what it just met. */}
              <span>
                Guardar “{preview.nickname_from_name}” como apelido de “{person.name}”, para
                o bot não cadastrar essa pessoa de novo
              </span>
            </label>
          )}
        </>
      )}
    </MergePanel>
  )
}
