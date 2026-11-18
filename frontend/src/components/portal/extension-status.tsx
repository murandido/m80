"use client"

import { useSyncExternalStore } from "react"
import { CheckCircle2, CircleDashed } from "lucide-react"
import { challengeBridgeReady } from "@/lib/smiles-challenge-extension"

// The extension marks the page once, at load, and never unmarks it: nothing to subscribe to.
function subscribeToNothing() {
  return () => {}
}

// Whether this browser has the extension, read from the mark its bridge puts on the page. Both
// builds (the team's and M80 Extrato) set it, which is right: either one connects the account.
export function ExtensionStatus() {
  const installed = useSyncExternalStore(subscribeToNothing, challengeBridgeReady, () => null)

  if (installed === null) return null
  return installed ? (
    <div className="flex items-center gap-2 rounded-md border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800">
      <CheckCircle2 className="h-4 w-4 shrink-0" />
      A extensão já está instalada neste navegador. Volte para a sua conta e clique em Conectar.
    </div>
  ) : (
    <div className="flex items-center gap-2 rounded-md border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700">
      <CircleDashed className="h-4 w-4 shrink-0" />
      A extensão ainda não está instalada neste navegador. Siga os passos abaixo.
    </div>
  )
}
