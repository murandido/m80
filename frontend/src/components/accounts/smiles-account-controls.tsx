"use client"

import { useCallback, useEffect, useRef, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { formatDistanceToNow } from "date-fns"
import { ptBR } from "date-fns/locale"
import { toast } from "sonner"
import { AlertTriangle, FileText, Link2, Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { startSmilesLoginAction } from "@/actions/statement"
import { statementSyncStateSchema, type SmilesAuthenticator, type StatementSyncState } from "@/schemas/inventory"
import { SmilesConnectDialog } from "@/components/accounts/smiles-connect-dialog"
import { Account } from "@/schemas/account"

// The stable identifier of the Smiles program is its code, not its name or id.
const SMILES_PROGRAM_CODE = "SMILES"

// How often the card re-reads the sync state after a connect. The first read is a handful of
// HTTP calls, so a few seconds is enough to catch it finishing.
const POLL_INTERVAL_MS = 4000

// How long to keep polling for that first read before giving up on it. It normally lands in
// seconds; past this the card just shows what it has, and a reload or the ticker will catch up.
const POLL_TIMEOUT_MS = 60_000

function formatPoints(points: number): string {
  return points.toLocaleString("pt-BR")
}

interface SmilesAccountControlsProps {
  account: Account
}

// The Smiles strip on the account card: connect the account, or show the synced balance with
// "atualizado há…" and, on an account that tracks its statement, a link to the statement page.
// Rendered for every Smiles account.
//
// Unlike the Azul strip there is no "conectando" to wait on: every step of a Smiles connect
// answers within its own request, and what takes time is the operator reading a code off their
// phone. What this does poll for is the first statement read, which runs in the background once
// the code is accepted.
export function SmilesAccountControls({ account }: SmilesAccountControlsProps) {
  const router = useRouter()
  const [isStarting, startLogin] = useTransition()
  const [status, setStatus] = useState<StatementSyncState | null>(null)
  const [authenticators, setAuthenticators] = useState<SmilesAuthenticator[] | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  // Set while the first read is expected, so the card polls for it and then stops.
  const [awaitingFirstSync, setAwaitingFirstSync] = useState(false)
  const pollingSince = useRef<number | null>(null)

  // Every Smiles account can connect: the balance is read for all of them. "Acompanhar extrato"
  // only decides whether the statement link shows - see SyncAccount on the backend.
  const enabled = account.loyalty_program.code === SMILES_PROGRAM_CODE

  const fetchStatus = useCallback(async () => {
    try {
      const response = await fetch(`/api/accounts/${account.id}/smiles-status`, {
        cache: "no-store",
      })
      if (!response.ok) return null
      const parsed = statementSyncStateSchema.safeParse(await response.json())
      if (!parsed.success) return null
      setStatus(parsed.data)
      return parsed.data
    } catch {
      // A failed poll is not worth a toast: the next tick tries again, and the card keeps
      // showing the last known state rather than flashing an error.
      return null
    }
  }, [account.id])

  // Read the badge once on mount for a connected account, so a session that has since been
  // refused shows up as "precisa de um código novo" rather than as a stale balance.
  useEffect(() => {
    if (!enabled || !account.smiles_connected) return
    let active = true
    fetch(`/api/accounts/${account.id}/smiles-status`, { cache: "no-store" })
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
  }, [enabled, account.smiles_connected, account.id])

  // After a connect, poll until the first read lands (state leaves "pending") or the window
  // closes. Then refresh, so the card picks up the balance the read wrote.
  useEffect(() => {
    if (!awaitingFirstSync) return
    pollingSince.current = Date.now()
    const poll = setInterval(() => {
      void fetchStatus().then((state) => {
        const timedOut = Date.now() - (pollingSince.current ?? 0) > POLL_TIMEOUT_MS
        if ((state && state.state !== "pending") || timedOut) {
          setAwaitingFirstSync(false)
          router.refresh()
        }
      })
    }, POLL_INTERVAL_MS)
    return () => clearInterval(poll)
  }, [awaitingFirstSync, fetchStatus, router])

  if (!enabled) {
    return null
  }

  function handleConnect() {
    startLogin(async () => {
      const result = await startSmilesLoginAction(account.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      if (result.connected) {
        // An account with no second factor: nothing to type.
        toast.success("Conta conectada.")
        setAwaitingFirstSync(true)
        return
      }
      setAuthenticators(result.authenticators)
      setDialogOpen(true)
    })
  }

  const connected = status?.connected ?? account.smiles_connected
  const needsCode = status?.state === "needs_mfa"
  const accountLabel = account.login ?? "a conta"

  const dialog = authenticators ? (
    <SmilesConnectDialog
      accountId={account.id}
      accountLabel={accountLabel}
      authenticators={authenticators}
      open={dialogOpen}
      onOpenChange={setDialogOpen}
      onConnected={() => {
        setAuthenticators(null)
        setAwaitingFirstSync(true)
      }}
      onExpired={() => setAuthenticators(null)}
    />
  ) : null

  // A session the airline no longer accepts. It is not an error to retry - only a new code
  // fixes it - so the card says exactly that instead of showing a failure count.
  if (connected && needsCode) {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-amber-50 px-4 py-2">
        <div className="flex flex-col">
          <span className="flex items-center gap-1.5 text-sm font-medium text-amber-800">
            <AlertTriangle className="h-3.5 w-3.5" />
            A Smiles pediu um código novo
          </span>
          <span className="text-xs text-amber-700">
            A sessão salva deixou de valer. Conecte de novo para receber um código.
          </span>
        </div>
        <Button variant="outline" size="sm" onClick={handleConnect} disabled={isStarting}>
          {isStarting ? (
            <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
          ) : (
            <Link2 className="mr-1 h-3.5 w-3.5" />
          )}
          Reconectar
        </Button>
        {dialog}
      </div>
    )
  }

  if (!connected) {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
        <span className="text-xs text-slate-500">Conta Smiles ainda não conectada</span>
        <Button
          variant="outline"
          size="sm"
          onClick={handleConnect}
          disabled={isStarting}
          className="border-orange-600 text-orange-600 hover:border-orange-500 hover:bg-orange-500 hover:text-white"
        >
          {isStarting ? (
            <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
          ) : (
            <Link2 className="mr-1 h-3.5 w-3.5" />
          )}
          Conectar
        </Button>
        {dialog}
      </div>
    )
  }

  // real_balance_points first: every round writes it, while last_balance_points only moves on a
  // round that read the statement, so on an account that does not track it it is empty or stale.
  const balancePoints = account.real_balance_points ?? status?.last_balance_points
  const balance =
    balancePoints != null ? `${formatPoints(balancePoints)} milhas` : "Saldo ainda não sincronizado"
  const lastSyncedAt = status?.last_synced_at ?? account.smiles_last_synced_at
  const syncedAt = lastSyncedAt
    ? `atualizado ${formatDistanceToNow(new Date(lastSyncedAt), {
        addSuffix: true,
        locale: ptBR,
      })}`
    : null

  return (
    <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
      <div className="flex flex-col">
        <span className="flex items-center gap-1.5 text-sm font-medium text-slate-800">
          {awaitingFirstSync && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          {awaitingFirstSync
            ? account.tracks_statement
              ? "Lendo o extrato…"
              : "Lendo o saldo…"
            : balance}
        </span>
        {!awaitingFirstSync && syncedAt && (
          <span className="text-xs text-slate-500">{syncedAt}</span>
        )}
      </div>
      {account.tracks_statement && (
        <Button
          asChild
          variant="outline"
          size="sm"
          className="border-slate-800 text-slate-800 hover:border-orange-500 hover:bg-orange-500 hover:text-white"
        >
          <Link href={`/dashboard/accounts/${account.id}/statement`}>
            <FileText className="mr-1 h-3.5 w-3.5" />
            Ver extrato
          </Link>
        </Button>
      )}
      {dialog}
    </div>
  )
}
