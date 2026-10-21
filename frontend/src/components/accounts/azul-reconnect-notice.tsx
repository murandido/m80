"use client"

import { useEffect, useRef, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { toast } from "sonner"
import { Loader2, Plug } from "lucide-react"
import { Button } from "@/components/ui/button"
import { connectAzulAccountAction } from "@/actions/statement"
import { statementSyncStateSchema, type StatementSyncState } from "@/schemas/inventory"
import { formatConnectingElapsed } from "@/lib/elapsed"

// How often the notice re-reads the connect status while a login is in flight - the same cadence
// as the account card.
const POLL_INTERVAL_MS = 5000

// What the operator is asked to do, per state. needs_reconnect is Azul refusing the stored key
// (in practice a password change), which saving the new password already fixes on its own;
// needs_attention is a run of failures whose cause is less certain.
const NOTICE_TEXT: Partial<Record<string, string>> = {
  needs_reconnect:
    "A Azul não reconhece mais esta conta — provavelmente a senha mudou. Atualize a senha no cadastro; o login é refeito sozinho ao salvar. Se a senha já estiver certa, clique em Reconectar.",
  needs_attention:
    "A sincronização falhou várias vezes seguidas. Confira o login e a senha salvos na conta e reconecte.",
}

interface AzulReconnectNoticeProps {
  accountId: string
  state: string
}

// The "this Azul account needs a login" notice for the statement page: the reason in plain words
// and a Reconectar button. The Reconectar on the inventory screen only shows for accounts that
// own their points, which left accounts administered for someone else with no way back.
//
// After the click it polls the status until the background login finishes, then refreshes the
// page so the badge and the statement reflect the outcome.
export function AzulReconnectNotice({ accountId, state }: AzulReconnectNoticeProps) {
  const router = useRouter()
  const [isStarting, startTransition] = useTransition()
  const [status, setStatus] = useState<StatementSyncState | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const wasConnecting = useRef(false)

  const connecting = status?.connecting ?? false

  useEffect(() => {
    if (!connecting) {
      if (wasConnecting.current) {
        wasConnecting.current = false
        if (status?.state === "pending" || status?.state === "ok") {
          toast.success("Conta reconectada.")
        } else {
          toast.error(
            status?.detail ? `Não consegui reconectar: ${status.detail}` : "Não consegui reconectar.",
          )
        }
        router.refresh()
      }
      return
    }
    wasConnecting.current = true
    const poll = setInterval(() => {
      fetch(`/api/accounts/${accountId}/azul-status`, { cache: "no-store" })
        .then((response) => (response.ok ? response.json() : null))
        .then((data) => {
          const parsed = statementSyncStateSchema.safeParse(data)
          if (parsed.success) setStatus(parsed.data)
        })
        // A failed poll is not worth a toast: the next tick tries again.
        .catch(() => {})
    }, POLL_INTERVAL_MS)
    const tick = setInterval(() => setNow(Date.now()), 1000)
    return () => {
      clearInterval(poll)
      clearInterval(tick)
    }
  }, [connecting, accountId, router, status?.state, status?.detail])

  const text = NOTICE_TEXT[state]
  if (!text) {
    return null
  }

  function reconnect() {
    startTransition(async () => {
      const result = await connectAzulAccountAction(accountId)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.info("Login iniciado. Isso pode levar alguns minutos…")
      // Show "conectando" at once; the poll takes over with the server's own state.
      setStatus((prev) => ({
        connected: prev?.connected ?? true,
        connecting: true,
        connecting_since: new Date().toISOString(),
        state: prev?.state ?? "pending",
        last_synced_at: prev?.last_synced_at,
        last_balance_points: prev?.last_balance_points,
        consecutive_failures: prev?.consecutive_failures ?? 0,
        detail: prev?.detail,
        updated_at: prev?.updated_at,
      }))
      setNow(Date.now())
    })
  }

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2">
      <p className="flex-1 text-xs text-rose-800">{text}</p>
      {connecting ? (
        <span className="flex items-center gap-1.5 text-xs font-medium text-sky-700">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          Conectando…{" "}
          {status?.connecting_since ? formatConnectingElapsed(status.connecting_since, now) : ""}
        </span>
      ) : (
        <Button variant="outline" size="sm" onClick={reconnect} disabled={isStarting}>
          {isStarting ? <Loader2 className="size-4 animate-spin" /> : <Plug className="size-4" />}
          Reconectar
        </Button>
      )}
    </div>
  )
}
