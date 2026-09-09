"use client"

import { useState, useTransition } from "react"
import { toast } from "sonner"
import { Check, Pencil, Trash2, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import {
  createPointBatchOriginAction,
  deletePointBatchOriginAction,
  updatePointBatchOriginAction,
} from "@/actions/inventory"
import type { PointBatchOrigin } from "@/schemas/inventory"

export function PointBatchOriginsScreen({ origins }: { origins: PointBatchOrigin[] }) {
  const [pending, startTransition] = useTransition()
  const [newName, setNewName] = useState("")
  const [editing, setEditing] = useState<string | null>(null)
  const [draft, setDraft] = useState("")

  function create() {
    const name = newName.trim()
    if (!name) {
      toast.error("Informe o nome da origem.")
      return
    }
    startTransition(async () => {
      const result = await createPointBatchOriginAction(name)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Origem criada.")
      setNewName("")
    })
  }

  function rename(origin: PointBatchOrigin) {
    const name = draft.trim()
    if (!name || name === origin.name) {
      setEditing(null)
      return
    }
    startTransition(async () => {
      const result = await updatePointBatchOriginAction(origin.id, { name })
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Origem renomeada.")
      setEditing(null)
    })
  }

  function toggleActive(origin: PointBatchOrigin) {
    startTransition(async () => {
      const result = await updatePointBatchOriginAction(origin.id, { active: !origin.active })
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success(origin.active ? "Origem desativada." : "Origem reativada.")
    })
  }

  function remove(origin: PointBatchOrigin) {
    startTransition(async () => {
      const result = await deletePointBatchOriginAction(origin.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Origem excluída.")
    })
  }

  return (
    <div className="flex max-w-3xl flex-col gap-5">
      <div className="flex items-end gap-2">
        <div className="flex flex-1 flex-col gap-1.5">
          <label htmlFor="new-origin" className="text-xs text-slate-500">
            Nova origem
          </label>
          <Input
            id="new-origin"
            value={newName}
            placeholder="Ex: Smiles Clube, Itaú, Compra bonificada"
            onChange={(event) => setNewName(event.target.value)}
            onKeyDown={(event) => event.key === "Enter" && create()}
          />
        </div>
        <Button onClick={create} disabled={pending}>
          Adicionar
        </Button>
      </div>

      <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-slate-100 text-xs uppercase tracking-wide text-slate-600 dark:bg-slate-800 dark:text-slate-300">
              <th className="px-4 py-2 text-left font-medium">Origem</th>
              <th className="w-32 px-4 py-2 text-left font-medium">Ativa</th>
              <th className="w-24 px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {origins.length === 0 ? (
              <tr>
                <td colSpan={3} className="px-4 py-6 text-center text-slate-500">
                  Nenhuma origem cadastrada.
                </td>
              </tr>
            ) : (
              origins.map((origin) => (
                <tr key={origin.id} className="hover:bg-slate-50 dark:hover:bg-slate-900/40">
                  <td className="px-4 py-2">
                    {editing === origin.id ? (
                      <div className="flex items-center gap-1">
                        <Input
                          value={draft}
                          autoFocus
                          onChange={(event) => setDraft(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") rename(origin)
                            if (event.key === "Escape") setEditing(null)
                          }}
                          className="h-8"
                        />
                        <Button size="icon" variant="ghost" onClick={() => rename(origin)} aria-label="Salvar">
                          <Check className="size-4" />
                        </Button>
                        <Button size="icon" variant="ghost" onClick={() => setEditing(null)} aria-label="Cancelar">
                          <X className="size-4" />
                        </Button>
                      </div>
                    ) : (
                      <button
                        type="button"
                        className="flex items-center gap-2 text-left hover:underline"
                        onClick={() => {
                          setEditing(origin.id)
                          setDraft(origin.name)
                        }}
                      >
                        {origin.name}
                        {!origin.active && <Badge variant="secondary">Inativa</Badge>}
                        <Pencil className="size-3 text-slate-400" />
                      </button>
                    )}
                  </td>
                  <td className="px-4 py-2">
                    <Switch
                      checked={origin.active}
                      disabled={pending}
                      onCheckedChange={() => toggleActive(origin)}
                      aria-label={origin.active ? "Desativar origem" : "Reativar origem"}
                    />
                  </td>
                  <td className="px-4 py-2 text-right">
                    {/* Refused by the backend once any batch points at it, with a message
                        saying to deactivate instead - the batches keep their label. */}
                    <Button
                      size="icon"
                      variant="ghost"
                      disabled={pending}
                      onClick={() => remove(origin)}
                      aria-label="Excluir origem"
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <p className="text-xs text-slate-500">
        Origem já usada por algum lote não pode ser excluída — desative-a. Ela some do formulário de
        lançamento e continua nomeando os lotes antigos.
      </p>
    </div>
  )
}
