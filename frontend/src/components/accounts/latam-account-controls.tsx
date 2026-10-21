"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { formatDistanceToNow } from "date-fns"
import { ptBR } from "date-fns/locale"
import { toast } from "sonner"
import { AlertTriangle, Link2, Loader2, Unlink } from "lucide-react"
import { Button } from "@/components/ui/button"
import { disconnectLatamAccountAction } from "@/actions/latam"
import { LatamConnectDialog } from "@/components/accounts/latam-connect-dialog"
import { Account } from "@/schemas/account"

// The stable identifier of the LATAM program is its code, not its name or id.
const LATAM_PROGRAM_CODE = "LATAM"

// The LATAM strip on the account card: connect the account, or show its LATAM Pass balance with
// "atualizado há…". Balance only - LATAM has no statement in the panel. Rendered for every LATAM
// account.
//
// There is nothing to poll: the connect finishes in the tab the handler on the operator's
// computer opens, which reads the balance once before it answers, and the ticker keeps it
// current after that.
export function LatamAccountControls({ account }: { account: Account }) {
  const router = useRouter()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [isDisconnecting, startDisconnecting] = useTransition()

  if (account.loyalty_program.code !== LATAM_PROGRAM_CODE) {
    return null
  }

  const accountLabel = account.login ?? "a conta"
  const dialog = (
    <LatamConnectDialog
      accountId={account.id}
      accountLabel={accountLabel}
      open={dialogOpen}
      onOpenChange={setDialogOpen}
    />
  )

  const connectButton = (label: string) => (
    <Button
      variant="outline"
      size="sm"
      onClick={() => setDialogOpen(true)}
      className="border-rose-600 text-rose-600 hover:border-rose-500 hover:bg-rose-500 hover:text-white"
    >
      <Link2 className="mr-1 h-3.5 w-3.5" />
      {label}
    </Button>
  )

  if (!account.latam_connected) {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
        <span className="text-xs text-slate-500">Conta LATAM ainda não conectada</span>
        {connectButton("Conectar")}
        {dialog}
      </div>
    )
  }

  // A session LATAM no longer accepts, or reads failing for a while: either way the fix is a new
  // login, so it is offered right here.
  const state = account.latam_sync_state
  if (state === "needs_login" || state === "needs_attention") {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-amber-50 px-4 py-2">
        <div className="flex flex-col">
          <span className="flex items-center gap-1.5 text-sm font-medium text-amber-800">
            <AlertTriangle className="h-3.5 w-3.5" />
            {state === "needs_login"
              ? "A LATAM pediu um login novo"
              : "A leitura do saldo está falhando"}
          </span>
          <span className="text-xs text-amber-700">Conecte de novo para voltar a ler o saldo.</span>
        </div>
        {connectButton("Reconectar")}
        {dialog}
      </div>
    )
  }

  function handleDisconnect() {
    startDisconnecting(async () => {
      const result = await disconnectLatamAccountAction(account.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Conta LATAM desconectada.")
      router.refresh()
    })
  }

  const balance =
    account.real_balance_points != null
      ? `${account.real_balance_points.toLocaleString("pt-BR")} milhas`
      : "Saldo ainda não lido"
  const syncedAt = account.latam_last_synced_at
    ? `atualizado ${formatDistanceToNow(new Date(account.latam_last_synced_at), {
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
        variant="ghost"
        size="sm"
        onClick={handleDisconnect}
        disabled={isDisconnecting}
        title="Desconectar a conta LATAM"
        className="text-slate-500 hover:text-rose-600"
      >
        {isDisconnecting ? (
          <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
        ) : (
          <Unlink className="mr-1 h-3.5 w-3.5" />
        )}
        Desconectar
      </Button>
    </div>
  )
}
