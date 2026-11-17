"use client"

import { useEffect, useRef, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { toast } from "sonner"
import { KeyRound, Loader2, Plug } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { connectPortalAzulAction, updatePortalPasswordAction } from "@/actions/portal"
import { statementSyncStateSchema, type StatementSyncKind, type SyncedProgram } from "@/schemas/inventory"

// Same cadence as the staff reconnect notice.
const POLL_INTERVAL_MS = 5000

// States in which the account will not sync until somebody acts.
const BROKEN_STATES: StatementSyncKind[] = ["needs_reconnect", "needs_attention", "needs_mfa", "error"]

interface PortalConnectPanelProps {
  accountId: string
  program: SyncedProgram
  connected: boolean
  state: StatementSyncKind
  hasPassword: boolean
}

// What a statement client can do to keep their account syncing: save a new airline password
// and, on Azul, start the login themselves. Smiles also needs a verification code that can only
// be requested from the team's browser extension, so for Smiles the panel says to call the team
// instead of offering a button that could never finish.
export function PortalConnectPanel({
  accountId,
  program,
  connected,
  state,
  hasPassword,
}: PortalConnectPanelProps) {
  const router = useRouter()
  const needsAction = !connected || BROKEN_STATES.includes(state)
  const [showPasswordForm, setShowPasswordForm] = useState(needsAction && !hasPassword)
  const [password, setPassword] = useState("")
  const [connecting, setConnecting] = useState(false)
  const [isPending, startTransition] = useTransition()
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // While an Azul login runs in the background, ask the status route until it settles, then
  // refresh the page so the list, the badge and the statement show the outcome.
  useEffect(() => {
    if (!connecting) return
    pollRef.current = setInterval(async () => {
      const res = await fetch(`/api/portal/accounts/${accountId}/status`, { cache: "no-store" })
      const body = await res.json().catch(() => null)
      const parsed = statementSyncStateSchema.safeParse(body)
      if (!parsed.success || parsed.data.connecting) return
      setConnecting(false)
      if (parsed.data.state === "ok" || parsed.data.state === "pending") {
        toast.success("Conta conectada. O extrato aparece assim que a leitura terminar.")
      } else {
        toast.error("Não consegui conectar. Confira a senha salva e tente de novo.")
      }
      router.refresh()
    }, POLL_INTERVAL_MS)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [connecting, accountId, router])

  function savePassword() {
    if (!password.trim()) {
      toast.error("Digite a senha.")
      return
    }
    startTransition(async () => {
      const result = await updatePortalPasswordAction(accountId, password)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      setPassword("")
      setShowPasswordForm(false)
      toast.success(
        program === "azul"
          ? "Senha salva. Clique em Conectar para entrar com ela."
          : "Senha salva. Avise a equipe M80 para concluir a conexão com o código da Smiles.",
      )
      router.refresh()
    })
  }

  function connectAzul() {
    startTransition(async () => {
      const result = await connectPortalAzulAction(accountId)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      setConnecting(true)
    })
  }

  return (
    <div className="flex flex-col gap-3 rounded-md border border-slate-200 p-4">
      {needsAction && (
        <p className="text-sm text-slate-700">
          {program === "azul"
            ? "Esta conta precisa ser conectada para o extrato atualizar. Se a senha da Azul mudou, salve a nova antes de conectar."
            : "Esta conta precisa de um código de verificação da Smiles para voltar a sincronizar. Se a senha mudou, salve a nova aqui e avise a equipe M80 para enviar o código."}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2">
        {program === "azul" && (
          <Button
            size="sm"
            variant={needsAction ? "default" : "outline"}
            disabled={isPending || connecting || !hasPassword}
            onClick={connectAzul}
            title={hasPassword ? undefined : "Salve a senha da Azul primeiro"}
          >
            {connecting ? (
              <Loader2 className="mr-1 h-4 w-4 animate-spin" />
            ) : (
              <Plug className="mr-1 h-4 w-4" />
            )}
            {connecting ? "Conectando… pode levar alguns minutos" : needsAction ? "Conectar" : "Reconectar"}
          </Button>
        )}
        <Button
          size="sm"
          variant="ghost"
          className="text-slate-600"
          disabled={isPending}
          onClick={() => setShowPasswordForm((open) => !open)}
        >
          <KeyRound className="mr-1 h-4 w-4" />
          {showPasswordForm ? "Cancelar" : "Trocar senha da conta"}
        </Button>
      </div>

      {showPasswordForm && (
        <form
          className="flex flex-wrap items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            savePassword()
          }}
        >
          <Input
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            placeholder={program === "azul" ? "Senha da Azul" : "Senha da Smiles"}
            className="max-w-xs"
            disabled={isPending}
          />
          <Button type="submit" size="sm" disabled={isPending}>
            {isPending && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
            Salvar senha
          </Button>
        </form>
      )}
    </div>
  )
}
