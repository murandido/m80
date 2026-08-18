"use client"

import { useEffect, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Merge, Plus, RefreshCw, Trash2, UserCheck } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import type { Account, AccountBeneficiary } from "@/schemas/account"
import {
  createAccountBeneficiaryAction,
  deleteAccountBeneficiaryAction,
  getAccountBeneficiariesAction,
  mergeAccountBeneficiariesAction,
  reconcileAccountBeneficiariesAction,
  updateAccountBeneficiaryAction,
} from "@/actions/account"

interface AccountBeneficiariesPanelProps {
  account: Account
  isOpen: boolean
  // Reconciling throws away every manual correction, so only an admin is offered it - the
  // API refuses it for anyone else regardless.
  canReconcile: boolean
}

function formatDate(value: string | null | undefined): string {
  if (!value) return "—"
  return new Date(value).toLocaleDateString("pt-BR")
}

export function AccountBeneficiariesPanel({
  account,
  isOpen,
  canReconcile,
}: AccountBeneficiariesPanelProps) {
  const router = useRouter()
  const [beneficiaries, setBeneficiaries] = useState<AccountBeneficiary[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  const [newName, setNewName] = useState("")
  const [newCPF, setNewCPF] = useState("")
  // The row currently being merged into another, if any.
  const [mergeSourceId, setMergeSourceId] = useState<string | null>(null)

  // Loaded on open rather than with the page: an account with years of history has
  // hundreds of these, and the accounts screen renders every account at once.
  useEffect(() => {
    if (!isOpen) return
    let cancelled = false

    setIsLoading(true)
    getAccountBeneficiariesAction(account.id).then((result) => {
      if (cancelled) return
      setIsLoading(false)
      if (result.error) {
        setError(result.error)
        return
      }
      setError(null)
      setBeneficiaries(result.beneficiaries ?? [])
    })

    return () => {
      cancelled = true
    }
  }, [isOpen, account.id])

  function refresh() {
    getAccountBeneficiariesAction(account.id).then((result) => {
      if (result.beneficiaries) setBeneficiaries(result.beneficiaries)
    })
    // The card's counter is server-rendered, so it only moves on a refresh.
    router.refresh()
  }

  function run(action: () => Promise<{ error?: string; success?: true }>) {
    startTransition(async () => {
      const result = await action()
      if (result.error) {
        setError(result.error)
        return
      }
      setError(null)
      refresh()
    })
  }

  function handleAdd() {
    if (!newName.trim()) return
    run(async () => {
      const result = await createAccountBeneficiaryAction(account.id, {
        name: newName.trim(),
        cpf: newCPF.trim() || undefined,
      })
      if (!result.error) {
        setNewName("")
        setNewCPF("")
      }
      return result
    })
  }

  function handleMerge(survivorId: string) {
    if (!mergeSourceId || mergeSourceId === survivorId) return
    const sourceId = mergeSourceId
    setMergeSourceId(null)
    run(() => mergeAccountBeneficiariesAction(account.id, survivorId, sourceId))
  }

  return (
    <div
      className={cn(
        "grid transition-all duration-300 ease-in-out",
        isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
    >
      <div className="overflow-hidden">
        <div className="rounded-b-md border border-t-0 border-slate-200 p-4">
          <div className="mb-4">
            <h2 className="text-sm font-semibold text-slate-800">
              Pessoas emitidas nesta conta
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Montado a partir das emissões. Quem não tem CPF é identificado pelo nome, então
              a mesma pessoa pode aparecer duas vezes — junte as linhas quando isso acontecer.
              O CPF informado aqui não é guardado: o sistema só grava a máscara e uma
              impressão digital para reconhecer a pessoa depois.
            </p>
          </div>

          {error && (
            <p className="mb-3 text-xs text-red-600" role="alert">
              {error}
            </p>
          )}

          {isLoading ? (
            <div className="flex justify-center py-4">
              <Spinner />
            </div>
          ) : beneficiaries.length === 0 ? (
            <p className="py-2 text-sm text-slate-500">
              Nenhuma pessoa registrada nesta conta ainda.
            </p>
          ) : (
            <ul className="flex flex-col divide-y divide-slate-100">
              {beneficiaries.map((beneficiary) => (
                <BeneficiaryRow
                  key={beneficiary.id}
                  accountId={account.id}
                  beneficiary={beneficiary}
                  isPending={isPending}
                  isMergeSource={mergeSourceId === beneficiary.id}
                  isMergeTarget={mergeSourceId !== null && mergeSourceId !== beneficiary.id}
                  onStartMerge={() => setMergeSourceId(beneficiary.id)}
                  onCancelMerge={() => setMergeSourceId(null)}
                  onMergeInto={() => handleMerge(beneficiary.id)}
                  onChanged={refresh}
                  onError={setError}
                />
              ))}
            </ul>
          )}

          <Separator className="my-4" />

          <div className="flex flex-wrap items-end gap-2">
            <div className="flex flex-1 flex-col gap-1">
              <Label htmlFor={`beneficiary-name-${account.id}`} className="text-xs text-slate-500">
                Nome
              </Label>
              <Input
                id={`beneficiary-name-${account.id}`}
                value={newName}
                onChange={(event) => setNewName(event.target.value)}
                placeholder="Quem foi emitido"
                className="bg-slate-100"
                disabled={isPending}
              />
            </div>
            <div className="flex w-44 flex-col gap-1">
              <Label htmlFor={`beneficiary-cpf-${account.id}`} className="text-xs text-slate-500">
                CPF (opcional)
              </Label>
              <Input
                id={`beneficiary-cpf-${account.id}`}
                value={newCPF}
                onChange={(event) => setNewCPF(event.target.value)}
                placeholder="000.000.000-00"
                className="bg-slate-100"
                disabled={isPending}
              />
            </div>
            <Button type="button" onClick={handleAdd} disabled={isPending || !newName.trim()}>
              <Plus className="h-4 w-4" />
              Adicionar
            </Button>

            {canReconcile && (
              <Button
                type="button"
                variant="outline"
                disabled={isPending}
                title="Descarta os ajustes manuais e reconstrói a lista a partir das emissões desta conta. Não tem desfazer."
                onClick={() => run(() => reconcileAccountBeneficiariesAction(account.id))}
                className="ml-auto"
              >
                <RefreshCw className="h-4 w-4" />
                Reconstruir pelas emissões
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

interface BeneficiaryRowProps {
  accountId: string
  beneficiary: AccountBeneficiary
  isPending: boolean
  isMergeSource: boolean
  isMergeTarget: boolean
  onStartMerge: () => void
  onCancelMerge: () => void
  onMergeInto: () => void
  onChanged: () => void
  onError: (message: string) => void
}

function BeneficiaryRow({
  accountId,
  beneficiary,
  isPending,
  isMergeSource,
  isMergeTarget,
  onStartMerge,
  onCancelMerge,
  onMergeInto,
  onChanged,
  onError,
}: BeneficiaryRowProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [name, setName] = useState(beneficiary.name)
  const [cpf, setCPF] = useState("")

  function save() {
    updateAccountBeneficiaryAction(accountId, beneficiary.id, {
      name: name.trim() || undefined,
      cpf: cpf.trim() || undefined,
    }).then((result) => {
      if (result.error) {
        onError(result.error)
        return
      }
      setIsEditing(false)
      setCPF("")
      onChanged()
    })
  }

  function toggleHolder() {
    updateAccountBeneficiaryAction(accountId, beneficiary.id, {
      is_holder: !beneficiary.is_holder,
    }).then((result) => {
      if (result.error) {
        onError(result.error)
        return
      }
      onChanged()
    })
  }

  if (isEditing) {
    return (
      <li className="flex flex-wrap items-end gap-2 py-2">
        <Input
          value={name}
          onChange={(event) => setName(event.target.value)}
          className="flex-1 bg-slate-100"
          aria-label="Nome"
        />
        <Input
          value={cpf}
          onChange={(event) => setCPF(event.target.value)}
          placeholder={beneficiary.cpf_masked ?? "CPF (opcional)"}
          className="w-44 bg-slate-100"
          aria-label="CPF"
        />
        <Button type="button" size="sm" onClick={save} disabled={isPending}>
          Salvar
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={() => setIsEditing(false)}>
          Cancelar
        </Button>
      </li>
    )
  }

  return (
    <li
      className={cn(
        "flex items-center justify-between gap-2 py-2",
        isMergeSource && "bg-amber-50",
      )}
    >
      <div className="flex min-w-0 flex-col">
        <span className="truncate text-sm text-slate-800">
          {beneficiary.name}
          {beneficiary.is_holder && (
            <span className="ml-2 text-xs text-slate-500">titular, não conta no limite</span>
          )}
        </span>
        <span className="text-xs text-slate-500">
          {beneficiary.has_cpf ? beneficiary.cpf_masked : "sem CPF, casado pelo nome"} ·{" "}
          {beneficiary.uses === 1 ? "1 emissão" : `${beneficiary.uses} emissões`} · última em{" "}
          {formatDate(beneficiary.last_used_at)}
        </span>
      </div>

      <div className="flex shrink-0 gap-1">
        {isMergeTarget ? (
          <Button type="button" size="sm" onClick={onMergeInto} disabled={isPending}>
            Juntar aqui
          </Button>
        ) : isMergeSource ? (
          <Button type="button" size="sm" variant="ghost" onClick={onCancelMerge}>
            Cancelar
          </Button>
        ) : (
          <>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              title={
                beneficiary.is_holder
                  ? "Voltar a contar esta pessoa no limite"
                  : "Marcar como titular da conta — o titular não consome vaga"
              }
              aria-label="Alternar titular"
              onClick={toggleHolder}
              disabled={isPending}
              className={beneficiary.is_holder ? "text-cyan-600" : "text-slate-500"}
            >
              <UserCheck className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              title="Juntar com outra linha — para quando a mesma pessoa entrou duas vezes"
              aria-label="Juntar beneficiário"
              onClick={onStartMerge}
              disabled={isPending}
              className="text-slate-500"
            >
              <Merge className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setIsEditing(true)}
              disabled={isPending}
              className="text-slate-500"
            >
              Corrigir
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              title="Remover — use para quem entrou errado. Cancelar uma emissão não devolve a vaga."
              aria-label="Remover beneficiário"
              onClick={() =>
                deleteAccountBeneficiaryAction(accountId, beneficiary.id).then((result) => {
                  if (result.error) {
                    onError(result.error)
                    return
                  }
                  onChanged()
                })
              }
              disabled={isPending}
              className="text-slate-500 hover:text-red-600"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </>
        )}
      </div>
    </li>
  )
}
