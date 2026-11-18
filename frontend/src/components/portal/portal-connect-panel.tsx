"use client"

import { useEffect, useRef, useState, useSyncExternalStore, useTransition } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { toast } from "sonner"
import { Globe, KeyRound, Loader2, Plug, Puzzle } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { connectPortalAzulAction, updatePortalPasswordAction } from "@/actions/portal"
import { startSmilesLoginAction, storeAzulBrowserKeyAction } from "@/actions/statement"
import { SmilesConnectDialog } from "@/components/accounts/smiles-connect-dialog"
import { azulConnectBridgeReady, requestAzulBrowserConnect } from "@/lib/azul-connect-extension"
import { challengeBridgeReady } from "@/lib/smiles-challenge-extension"
import {
  statementSyncStateSchema,
  type SmilesAuthenticator,
  type StatementSyncKind,
  type SyncedProgram,
} from "@/schemas/inventory"

// Same cadence as the staff reconnect notice.
const POLL_INTERVAL_MS = 5000

// States in which the account will not sync until somebody acts.
const BROKEN_STATES: StatementSyncKind[] = ["needs_reconnect", "needs_attention", "needs_mfa", "error"]

// The extension marks the page once, at load, and never unmarks it: nothing to subscribe to.
function subscribeToNothing() {
  return () => {}
}

interface PortalConnectPanelProps {
  accountId: string
  program: SyncedProgram
  // The account's CPF at the airline; the Azul browser login is checked against it.
  login: string | null
  holderName: string
  connected: boolean
  state: StatementSyncKind
  hasPassword: boolean
}

// What a statement client can do to keep their account syncing, without the team: save a new
// airline password and connect. Azul connects on the server (Conectar) or through a login the
// client makes in their own browser (Pelo navegador); Smiles always needs a verification code
// that only a real browser can request. Both browser paths go through the M80 Extrato extension,
// so without it the panel points at the install guide instead of a button that cannot finish.
export function PortalConnectPanel({
  accountId,
  program,
  login,
  holderName,
  connected,
  state,
  hasPassword,
}: PortalConnectPanelProps) {
  const router = useRouter()
  const needsAction = !connected || BROKEN_STATES.includes(state)
  const [showPasswordForm, setShowPasswordForm] = useState(needsAction && !hasPassword)
  const [password, setPassword] = useState("")
  const [connecting, setConnecting] = useState(false)
  const [browserConnecting, setBrowserConnecting] = useState(false)
  const [missingExtension, setMissingExtension] = useState(false)
  const [authenticators, setAuthenticators] = useState<SmilesAuthenticator[] | null>(null)
  const [isPending, startTransition] = useTransition()
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Read on the client only: the server render never sees the extension's mark.
  const extensionReady = useSyncExternalStore(
    subscribeToNothing,
    () => (program === "azul" ? azulConnectBridgeReady() : challengeBridgeReady()),
    () => false,
  )

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
        toast.error("Não consegui conectar. Confira a senha salva ou tente Pelo navegador.")
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
      // A parked Azul account starts its login on its own when the password changes.
      toast.success(
        program === "azul"
          ? "Senha salva. Se a conta estava desconectada, a conexão começa sozinha; senão, clique em Conectar."
          : "Senha salva. Agora clique em Conectar para receber o código da Smiles.",
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

  async function connectAzulInBrowser() {
    if (!azulConnectBridgeReady()) {
      setMissingExtension(true)
      return
    }
    setBrowserConnecting(true)
    try {
      const answer = await requestAzulBrowserConnect(accountId, login ?? "")
      if ("error" in answer) {
        toast.error(answer.error)
        return
      }
      const stored = await storeAzulBrowserKeyAction(accountId, answer.customerKey, answer.document, "portal")
      if ("error" in stored) {
        toast.error(stored.error)
        return
      }
      toast.success("Conta conectada. O extrato aparece assim que a leitura terminar.")
      router.refresh()
    } finally {
      setBrowserConnecting(false)
    }
  }

  function connectSmiles() {
    // Checked before the login: without the extension the code step cannot happen, and a login
    // started for nothing still counts against the holder at Smiles.
    if (!challengeBridgeReady()) {
      setMissingExtension(true)
      return
    }
    startTransition(async () => {
      const result = await startSmilesLoginAction(accountId, "portal")
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      if (result.connected) {
        toast.success("Conta conectada. O extrato aparece assim que a leitura terminar.")
        router.refresh()
        return
      }
      setAuthenticators(result.authenticators)
    })
  }

  const busy = isPending || connecting || browserConnecting

  return (
    <div className="flex flex-col gap-3 rounded-md border border-slate-200 p-4">
      {needsAction && (
        <p className="text-sm text-slate-700">
          {program === "azul"
            ? "Esta conta precisa ser conectada para o extrato atualizar. Se a senha da Azul mudou, salve a nova antes de conectar."
            : "Esta conta precisa ser conectada para o extrato atualizar. A Smiles vai mandar um código por SMS ou e-mail. Se a senha da Smiles mudou, salve a nova antes de conectar."}
        </p>
      )}

      {browserConnecting && (
        <div className="flex items-start gap-2 rounded-md bg-sky-50 p-3 text-sm text-sky-800">
          <Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin" />
          <span>
            Abrimos o site da Azul numa nova aba. Clique em <strong>Entrar</strong>, faça o login com
            o seu CPF e a sua senha da Azul, e volte para esta página. A aba fecha sozinha quando a
            conexão terminar.
          </span>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        {program === "azul" ? (
          <>
            <Button
              size="sm"
              variant={needsAction ? "default" : "outline"}
              disabled={busy || !hasPassword}
              onClick={connectAzul}
              title={hasPassword ? undefined : "Salve a senha da Azul primeiro"}
            >
              {connecting ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <Plug className="mr-1 h-4 w-4" />}
              {connecting ? "Conectando… pode levar alguns minutos" : needsAction ? "Conectar" : "Reconectar"}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => void connectAzulInBrowser()}
              title="Abre o site da Azul numa aba para você fazer o login. Use se o Conectar não funcionar."
            >
              <Globe className="mr-1 h-4 w-4" />
              Pelo navegador
            </Button>
          </>
        ) : (
          <Button
            size="sm"
            variant={needsAction ? "default" : "outline"}
            disabled={busy || !hasPassword}
            onClick={connectSmiles}
            title={hasPassword ? undefined : "Salve a senha da Smiles primeiro"}
          >
            {isPending ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <Plug className="mr-1 h-4 w-4" />}
            {needsAction ? "Conectar" : "Reconectar"}
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
          {showPasswordForm ? "Cancelar" : program === "azul" ? "Trocar senha da Azul" : "Trocar senha da Smiles"}
        </Button>
      </div>

      {(missingExtension || (!extensionReady && program === "smiles" && needsAction)) && (
        <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
          <Puzzle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>
            {program === "azul"
              ? "Para conectar pelo navegador, instale a extensão M80 Extrato no Chrome e recarregue esta página."
              : "Para receber o código da Smiles, instale a extensão M80 Extrato no Chrome e recarregue esta página."}{" "}
            <Link href="/portal/extensao" className="font-medium underline">
              Como instalar
            </Link>
          </span>
        </div>
      )}

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
            placeholder={program === "azul" ? "Nova senha da Azul" : "Nova senha da Smiles"}
            className="max-w-xs"
            disabled={isPending}
          />
          <Button type="submit" size="sm" disabled={isPending}>
            {isPending && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
            Salvar senha
          </Button>
        </form>
      )}

      {program === "smiles" && (
        <SmilesConnectDialog
          area="portal"
          accountId={accountId}
          accountLabel={holderName}
          authenticators={authenticators ?? []}
          open={authenticators !== null}
          onOpenChange={(open) => {
            if (!open) setAuthenticators(null)
          }}
          onConnected={() => router.refresh()}
          onExpired={() => setAuthenticators(null)}
        />
      )}
    </div>
  )
}
