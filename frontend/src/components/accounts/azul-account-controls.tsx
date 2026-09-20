"use client"

import { useCallback, useEffect, useRef, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { formatDistanceToNow } from "date-fns"
import { ptBR } from "date-fns/locale"
import { toast } from "sonner"
import { FileText, Link2, Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { connectAzulAccountAction } from "@/actions/statement"
import { azulSyncStateSchema, type AzulSyncState } from "@/schemas/inventory"
import { formatConnectingElapsed } from "@/lib/elapsed"
import { Account } from "@/schemas/account"

// The stable identifier of the Azul program is its code, not its name or id (see the loyalty
// program branch key used elsewhere). Connect and the statement page are Azul-only; the balance
// itself is shown for any inventory account that has one.
const AZUL_PROGRAM_CODE = "AZUL"

// How often the card re-reads the connect status while a login is in flight. The login runs for a
// couple of minutes (edge retries), so a five-second poll is plenty to catch the finish without
// hammering the API.
const POLL_INTERVAL_MS = 5000

function formatPoints(points: number): string {
  return points.toLocaleString("pt-BR")
}

interface AzulAccountControlsProps {
  account: Account
}

// The Azul strip on the account card: connect the account (when it has no customerKey yet) or,
// once connected, show the synced balance with "atualizado há…" and a link to the statement
// page. Rendered only for Azul accounts that own their point inventory - every other account
// has no sync to offer.
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
  const [status, setStatus] = useState<AzulSyncState | null>(null)
  // A ticking clock, so the elapsed time next to "conectando" advances every second.
  const [now, setNow] = useState(() => Date.now())
  // Remembers we were mid-connect, so the moment the poll reports it finished we refresh the page
  // to pull the freshly synced balance into the card.
  const wasConnecting = useRef(false)

  const isAzul = account.loyalty_program.code === AZUL_PROGRAM_CODE
  const isInventory = account.owns_point_inventory
  const enabled = isAzul && isInventory

  const fetchStatus = useCallback(async () => {
    try {
      const response = await fetch(`/api/accounts/${account.id}/azul-status`, {
        cache: "no-store",
      })
      if (!response.ok) return
      const parsed = azulSyncStateSchema.safeParse(await response.json())
      if (parsed.success) setStatus(parsed.data)
    } catch {
      // A failed poll is not worth a toast: the next tick tries again, and the card keeps showing
      // the last known state rather than flashing an error mid-login.
    }
  }, [account.id])

  // On mount, learn whether a connect is already in flight - but only for a not-yet-connected
  // account, which is the one state where a login can be running or needed. A connected account
  // shows its balance straight from the server render, no request per card on every page load.
  // The fetch is inlined (rather than calling fetchStatus) so the setState lands inside the
  // promise callback, not the effect body, and a late response for an unmounted card is dropped.
  useEffect(() => {
    if (!enabled || account.azul_connected) return
    let active = true
    fetch(`/api/accounts/${account.id}/azul-status`, { cache: "no-store" })
      .then((response) => (response.ok ? response.json() : null))
      .then((data) => {
        if (!active) return
        const parsed = azulSyncStateSchema.safeParse(data)
        if (parsed.success) setStatus(parsed.data)
      })
      .catch(() => {})
    return () => {
      active = false
    }
  }, [enabled, account.azul_connected, account.id])

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
            O login pode levar alguns minutos. Pode sair da página — ele continua.
          </span>
        </div>
      </div>
    )
  }

  const connected = status?.connected ?? account.azul_connected

  if (!connected) {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
        <span className="text-xs text-slate-500">Conta Azul ainda não conectada</span>
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
    )
  }

  const balancePoints = status?.last_balance_points ?? account.real_balance_points
  const balance =
    balancePoints != null ? `${formatPoints(balancePoints)} pts` : "Saldo ainda não sincronizado"
  const lastSyncedAt = status?.last_synced_at ?? account.azul_last_synced_at
  const syncedAt = lastSyncedAt
    ? `atualizado ${formatDistanceToNow(new Date(lastSyncedAt), {
        addSuffix: true,
        locale: ptBR,
      })}`
    : null

  return (
    <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
      <div className="flex flex-col">
        <span className="text-sm font-medium text-slate-800">{balance}</span>
        {syncedAt && <span className="text-xs text-slate-500">{syncedAt}</span>}
      </div>
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
    </div>
  )
}
