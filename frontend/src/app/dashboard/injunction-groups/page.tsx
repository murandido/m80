import type { Metadata } from "next"
import { requireDev } from "@/lib/session"
import {
  getMicrosoftConnectionAction,
  injunctionFormOptionsAction,
  listInjunctionGroupsAction,
} from "@/actions/injunction-group"
import { MicrosoftConnectionCard } from "@/components/injunction-groups/microsoft-connection-card"
import { InjunctionGroupsPanel } from "@/components/injunction-groups/injunction-groups-panel"

export const metadata: Metadata = {
  title: "Tropas",
  description: "Grupos de contas liminares com planilha no OneDrive e grupo no WhatsApp.",
}

// What the Microsoft callback reports back in ?microsoft=.
const CALLBACK_MESSAGES: Record<string, { text: string; error: boolean }> = {
  ok: { text: "Conta Microsoft conectada.", error: false },
  cancelled: { text: "Login Microsoft cancelado.", error: true },
  error: { text: "Não foi possível concluir o login Microsoft. Tente de novo.", error: true },
}

export default async function InjunctionGroupsPage({
  searchParams,
}: {
  searchParams: Promise<{ microsoft?: string }>
}) {
  await requireDev()

  const [{ microsoft }, connectionResult, groupsResult, options] = await Promise.all([
    searchParams,
    getMicrosoftConnectionAction(),
    listInjunctionGroupsAction(),
    injunctionFormOptionsAction(),
  ])
  const connection = "data" in connectionResult ? (connectionResult.data ?? null) : null
  const groups = "data" in groupsResult ? (groupsResult.data ?? []) : []
  const callback = microsoft ? CALLBACK_MESSAGES[microsoft] : undefined

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Tropas</h1>
        {callback && (
          <p className={callback.error ? "text-sm text-destructive" : "text-sm text-emerald-700"}>{callback.text}</p>
        )}
        <MicrosoftConnectionCard connection={connection} />
        <InjunctionGroupsPanel groups={groups} options={options} />
      </div>
    </div>
  )
}
