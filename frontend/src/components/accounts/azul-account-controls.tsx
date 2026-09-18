"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { formatDistanceToNow } from "date-fns"
import { ptBR } from "date-fns/locale"
import { toast } from "sonner"
import { FileText, Link2, Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { connectAzulAccountAction } from "@/actions/statement"
import { Account } from "@/schemas/account"

// The stable identifier of the Azul program is its code, not its name or id (see the loyalty
// program branch key used elsewhere). Connect and the statement page are Azul-only; the balance
// itself is shown for any inventory account that has one.
const AZUL_PROGRAM_CODE = "AZUL"

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
export function AzulAccountControls({ account }: AzulAccountControlsProps) {
  const router = useRouter()
  const [isConnecting, startConnecting] = useTransition()
  // Kept locally so the button reflects "conectando" until the page revalidates, even though the
  // real progress lives in the sync-state badge on the statement page and the panel.
  const [started, setStarted] = useState(false)

  const isAzul = account.loyalty_program.code === AZUL_PROGRAM_CODE
  if (!isAzul || !account.owns_point_inventory) {
    return null
  }

  function handleConnect() {
    startConnecting(async () => {
      const result = await connectAzulAccountAction(account.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      setStarted(true)
      toast.success("Login iniciado. O saldo aparece assim que a conta conectar.")
      router.refresh()
    })
  }

  if (!account.azul_connected) {
    return (
      <div className="flex items-center justify-between gap-2 border-t border-slate-100 bg-slate-50 px-4 py-2">
        <span className="text-xs text-slate-500">Conta Azul ainda não conectada</span>
        <Button
          variant="outline"
          size="sm"
          onClick={handleConnect}
          disabled={isConnecting || started}
          className="border-sky-700 text-sky-700 hover:border-sky-600 hover:bg-sky-600 hover:text-white"
        >
          {isConnecting || started ? (
            <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
          ) : (
            <Link2 className="mr-1 h-3.5 w-3.5" />
          )}
          {started ? "Conectando…" : "Conectar"}
        </Button>
      </div>
    )
  }

  const balance =
    account.real_balance_points != null
      ? `${formatPoints(account.real_balance_points)} pts`
      : "Saldo ainda não sincronizado"
  const syncedAt = account.azul_last_synced_at
    ? `atualizado ${formatDistanceToNow(new Date(account.azul_last_synced_at), {
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
