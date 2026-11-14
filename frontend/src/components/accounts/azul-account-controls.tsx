"use client"

import { useCallback, useEffect, useRef, useState, useSyncExternalStore, useTransition } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { formatDistanceToNow } from "date-fns"
import { ptBR } from "date-fns/locale"
import { toast } from "sonner"
import { FileText, Globe, Link2, Loader2, Plug } from "lucide-react"
import { Button } from "@/components/ui/button"
import { connectAzulAccountAction, storeAzulBrowserKeyAction } from "@/actions/statement"
import { azulConnectBridgeReady, requestAzulBrowserConnect } from "@/lib/azul-connect-extension"
import { statementSyncStateSchema, type StatementSyncState } from "@/schemas/inventory"
import { formatConnectingElapsed } from "@/lib/elapsed"
import { formatSignedPaddedPoints } from "@/lib/points"
import { Account } from "@/schemas/account"

// The stable identifier of the Azul program is its code, not its name or id (see the loyalty
// program branch key used elsewhere). Connect and the statement page are Azul-only; the balance
// itself is shown for any inventory account that has one.
const AZUL_PROGRAM_CODE = "AZUL"

// How often the card re-reads the connect status while a login is in flight. The login runs for a
// couple of minutes (with retries), so a five-second poll is plenty to catch the finish without
// hammering the API.
const POLL_INTERVAL_MS = 5000

// The extension marks the document before the panel's scripts run and never unmarks it, so there
// is nothing to subscribe to; the server render has no extension.
const subscribeToNothing = () => () => {}

interface AzulAccountControlsProps {
  account: Account
}

// The Azul strip on the account card: connect the account (when it has no customerKey yet) or,
// once connected, show the synced balance with "atualizado há…" and, on an account that tracks
// its statement, a link to the statement page. Rendered for every Azul account.
//
// The "conectando" state is owned by the server (the in-flight connect tracker), not by this
// component: the card reads it back from GET /api/accounts/{id}/azul-status, so leaving the page,
// an F5, or reopening the accounts screen all recover the real state instead of losing it. While a
// login is in flight the card polls until it finishes, then refreshes to pull in the new balance.
export function AzulAccountControls({ account }: AzulAccountControlsProps) {
  const router = useRouter()
  const [isConnecting, startConnecting] = useTransition()
  // The server-owned sync state, once fetched. Null before the first read; until then the card
  // falls back to what the account row already carried from the server render.
  const [status, setStatus] = useState<StatementSyncState | null>(null)
  // A ticking clock, so the elapsed time next to "conectando" advances every second.
  const [now, setNow] = useState(() => Date.now())
  // Remembers we were mid-connect, so the moment the poll reports it finished we refresh the page
  // to pull the freshly synced balance into the card.
  const wasConnecting = useRef(false)
  // When this tab last asked for a connect. A state detail written after it is the outcome of that
  // attempt, which is worth showing even on a state whose detail is usually a read error.
  const [connectRequestedAt, setConnectRequestedAt] = useState<number | null>(null)
  // The fallback: the operator logs in on the Azul site in a tab the extension opens. Offered only
  // when the extension is there to answer, and tracked here because it lives in this tab - a
  // person is typing in the other one.
  const bridgeReady = useSyncExternalStore(subscribeToNothing, azulConnectBridgeReady, () => false)
  const [browserConnecting, setBrowserConnecting] = useState(false)

  // Every Azul account can connect: the balance is read for all of them. "Acompanhar extrato"
  // only decides whether the statement link shows - see SyncAccount on the backend.
  const enabled = account.loyalty_program.code === AZUL_PROGRAM_CODE

  const fetchStatus = useCallback(async () => {
    try {
      const response = await fetch(`/api/accounts/${account.id}/azul-status`, {
        cache: "no-store",
      })
      if (!response.ok) return
      const parsed = statementSyncStateSchema.safeParse(await response.json())
      if (parsed.success) setStatus(parsed.data)
    } catch {
      // A failed poll is not worth a toast: the next tick tries again, and the card keeps showing
      // the last known state rather than flashing an error mid-login.
    }
  }, [account.id])

  // On mount, learn whether a connect is already in flight, and why the last one failed - but only
  // for an account that is not connected or is waiting for a login, the states where a login can
  // be running or needed. A healthy connected account shows its balance straight from the server
  // render, no request per card on every page load.
  // The fetch is inlined (rather than calling fetchStatus) so the setState lands inside the
  // promise callback, not the effect body, and a late response for an unmounted card is dropped.
  const waitingForLogin =
    account.azul_sync_state === "needs_reconnect" || account.azul_sync_state === "needs_attention"
  useEffect(() => {
    if (!enabled || (account.azul_connected && !waitingForLogin)) return
    let active = true
    fetch(`/api/accounts/${account.id}/azul-status`, { cache: "no-store" })
      .then((response) => (response.ok ? response.json() : null))
      .then((data) => {
        if (!active) return
        const parsed = statementSyncStateSchema.safeParse(data)
        if (parsed.success) setStatus(parsed.data)
      })
      .catch(() => {})
    return () => {
      active = false
    }
  }, [enabled, account.azul_connected, waitingForLogin, account.id])

  const connecting = status?.connecting ?? false

  // While connecting: poll the status and tick the elapsed clock. When it stops connecting after
  // having been, refresh so the (now connected) card reflects the new balance.
  useEffect(() => {
    if (!connecting) {
      if (wasConnecting.current) {
        wasConnecting.current = false
        router.refresh()
      }
      return
    }
    wasConnecting.current = true
    const poll = setInterval(() => void fetchStatus(), POLL_INTERVAL_MS)
    const tick = setInterval(() => setNow(Date.now()), 1000)
    return () => {
      clearInterval(poll)
      clearInterval(tick)
    }
  }, [connecting, fetchStatus, router])

  if (!enabled) {
    return null
  }

  function handleConnect() {
    startConnecting(async () => {
      const result = await connectAzulAccountAction(account.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Login iniciado. O saldo aparece assim que a conta conectar.")
      setConnectRequestedAt(Date.now())
      // Show "conectando" at once, then let the poll take over with the server's own timestamp.
      setStatus((prev) => ({
        connected: prev?.connected ?? account.azul_connected,
        connecting: true,
        connecting_since: new Date().toISOString(),
        state: prev?.state ?? "pending",
        last_synced_at: prev?.last_synced_at ?? account.azul_last_synced_at,
        last_balance_points: prev?.last_balance_points ?? account.real_balance_points,
        consecutive_failures: prev?.consecutive_failures ?? 0,
        detail: prev?.detail,
        updated_at: prev?.updated_at,
      }))
      setNow(Date.now())
    })
  }

  async function handleBrowserConnect() {
    setBrowserConnecting(true)
    try {
      const answer = await requestAzulBrowserConnect(account.id, account.login ?? "")
      if ("error" in answer) {
        toast.error(answer.error)
        return
      }
      const stored = await storeAzulBrowserKeyAction(account.id, answer.customerKey, answer.document)
      if ("error" in stored) {
        toast.error(stored.error)
        return
      }
      toast.success("Conta conectada pelo navegador. O saldo aparece em instantes.")
      await fetchStatus()
      router.refresh()
    } finally {
      setBrowserConnecting(false)
    }
  }

  const browserButton = bridgeReady ? (
    <Button
      variant="outline"
      size="sm"
      onClick={() => void handleBrowserConnect()}
      disabled={isConnecting}
      title="Abre a Azul numa aba para você fazer o login; use quando o Conectar automático não funcionar"
      className="border-slate-400 text-slate-700 hover:border-slate-600 hover:bg-slate-600 hover:text-white"
    >
      <Globe className="mr-1 h-3.5 w-3.5" />
      Pelo navegador
    </Button>
  ) : null

  if (browserConnecting) {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
        <div className="flex flex-col">
          <span className="flex items-center gap-1.5 text-sm font-medium text-sky-700">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            Aguardando o login na aba da Azul…
          </span>
          <span className="text-xs text-slate-500">
            Na aba que abriu, clique em Entrar no topo: o login e a senha entram sozinhos. Depois
            confirme. Para cancelar, feche a aba.
          </span>
        </div>
      </div>
    )
  }

  // A login in flight - whether this tab started it or it was already running when the card
  // mounted. Shown with a running timer so it reads as working, not stuck.
  if (connecting) {
    const since = status?.connecting_since
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
        <div className="flex flex-col">
          <span className="flex items-center gap-1.5 text-sm font-medium text-sky-700">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            Conectando… {since ? formatConnectingElapsed(since, now) : ""}
          </span>
          <span className="text-xs text-slate-500">
            O login pode levar alguns minutos, ou mais se houver outras contas na fila. Pode sair
            da página — ele continua.
          </span>
        </div>
      </div>
    )
  }

  const connected = status?.connected ?? account.azul_connected
  const syncState = status?.state ?? account.azul_sync_state

  // Why the last connect did not finish, as the backend wrote it for the operator (Portuguese,
  // with what to do). Shown where it can only be a connect outcome: a never-connected account or
  // one parked for a login - plus any state right after a connect from this tab, so a click that
  // failed always says why instead of the card silently going back to where it was.
  const answeredThisAttempt =
    connectRequestedAt != null &&
    status?.updated_at != null &&
    Date.parse(status.updated_at) >= connectRequestedAt
  const connectDetail =
    status?.detail && (!connected || syncState === "needs_reconnect" || answeredThisAttempt)
      ? status.detail
      : null
  // An account outcome (password, lock) is the operator's to fix; anything else is a wait.
  const accountProblem =
    syncState === "error" || syncState === "needs_attention" || syncState === "needs_reconnect"

  if (!connected) {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
        <div className="flex flex-col">
          <span className="text-xs text-slate-500">Conta Azul ainda não conectada</span>
          {connectDetail && (
            <span className={`text-xs ${accountProblem ? "text-rose-700" : "text-amber-700"}`}>
              {connectDetail}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {browserButton}
          <Button
            variant="outline"
            size="sm"
            onClick={handleConnect}
            disabled={isConnecting}
            className="border-sky-700 text-sky-700 hover:border-sky-600 hover:bg-sky-600 hover:text-white"
          >
            {isConnecting ? (
              <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Link2 className="mr-1 h-3.5 w-3.5" />
            )}
            Conectar
          </Button>
        </div>
      </div>
    )
  }

  // real_balance_points first: every round writes it, while last_balance_points only moves on a
  // round that read the statement, so on an account that does not track it it is empty or stale.
  const balancePoints = account.real_balance_points ?? status?.last_balance_points
  const balance =
    balancePoints != null ? `${formatSignedPaddedPoints(balancePoints)} pts` : "Saldo ainda não sincronizado"
  const lastSyncedAt = status?.last_synced_at ?? account.azul_last_synced_at
  const syncedAt = lastSyncedAt
    ? `atualizado ${formatDistanceToNow(new Date(lastSyncedAt), {
        addSuffix: true,
        locale: ptBR,
      })}`
    : null

  // A connected account whose sync stopped: Azul refused the key (needs_reconnect) or the rounds
  // keep failing (needs_attention). Either way the fix is a login, so offer it right here.
  const needsLogin = syncState === "needs_reconnect" || syncState === "needs_attention"

  return (
    <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
      <div className="flex flex-col">
        <span className="text-sm font-medium text-slate-800">{balance}</span>
        {syncedAt && <span className="text-xs text-slate-500">{syncedAt}</span>}
        {needsLogin && !connectDetail && (
          <span className="text-xs text-rose-700">
            {syncState === "needs_reconnect"
              ? "A Azul não reconhece mais a conta — atualize a senha"
              : "Sincronização falhando — confira a senha"}
          </span>
        )}
        {connectDetail && (
          <span className={`text-xs ${accountProblem ? "text-rose-700" : "text-amber-700"}`}>
            {connectDetail}
          </span>
        )}
      </div>
      {needsLogin && (
        <div className="ml-auto flex items-center gap-2">
          {browserButton}
          <Button
            variant="outline"
            size="sm"
            onClick={handleConnect}
            disabled={isConnecting}
            className="border-sky-700 text-sky-700 hover:border-sky-600 hover:bg-sky-600 hover:text-white"
          >
            {isConnecting ? (
              <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Plug className="mr-1 h-3.5 w-3.5" />
            )}
            Reconectar
          </Button>
        </div>
      )}
      {account.tracks_statement && (
        <Button
          asChild
          variant="outline"
          size="sm"
          className="border-slate-800 text-slate-800 hover:border-cyan-600 hover:bg-cyan-600 hover:text-white"
        >
          <Link href={`/dashboard/accounts/${account.id}/statement`}>
            <FileText className="mr-1 h-3.5 w-3.5" />
            Ver extrato
          </Link>
        </Button>
      )}
    </div>
  )
}
