import type { Metadata } from "next"
import Link from "next/link"
import { formatDistanceToNow } from "date-fns"
import { ptBR } from "date-fns/locale"
import { ChevronRight } from "lucide-react"
import { getPortalAccountsAction } from "@/actions/portal"
import { LoyaltyProgramBadge } from "@/components/accounts/loyalty-program-badge"
import { formatSignedPaddedPoints } from "@/lib/points"

export const metadata: Metadata = {
  title: "Suas contas · M80",
  description: "As contas Azul e Smiles cujo extrato você acompanha.",
}

export default async function PortalHomePage() {
  const result = await getPortalAccountsAction()

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 px-4 py-6">
      <h1 className="text-lg font-semibold text-slate-800">Suas contas</h1>

      {"error" in result ? (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
          {result.error}
        </div>
      ) : result.accounts.length === 0 ? (
        <div className="rounded-md border border-dashed border-slate-200 p-8 text-center text-sm text-slate-500">
          Nenhuma conta liberada para você ainda. Fale com a equipe M80.
        </div>
      ) : (
        <div className="flex flex-col divide-y divide-slate-100 rounded-md border border-slate-200">
          {result.accounts.map((account) => (
            <Link
              key={account.id}
              href={`/portal/accounts/${account.id}`}
              className="flex items-center justify-between gap-3 px-4 py-3 hover:bg-slate-50"
            >
              <div className="flex min-w-0 items-center gap-3">
                <LoyaltyProgramBadge
                  loyaltyProgram={{
                    id: account.program_id,
                    name: account.program_name,
                    code: account.program_code,
                    color: account.program_color,
                  }}
                />
                <div className="flex min-w-0 flex-col">
                  <span className="truncate text-sm font-medium text-slate-800">
                    {account.holder_name}
                  </span>
                  <span className="truncate text-xs text-slate-500">
                    {account.login ?? "Sem login"}
                    {" · "}
                    {account.last_synced_at
                      ? `sincronizado ${formatDistanceToNow(new Date(account.last_synced_at), {
                          addSuffix: true,
                          locale: ptBR,
                        })}`
                      : account.connected
                        ? "aguardando a primeira sincronização"
                        : "não conectada"}
                  </span>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <span className="text-sm font-medium tabular-nums text-slate-800">
                  {account.last_balance_points != null
                    ? `${formatSignedPaddedPoints(account.last_balance_points)} pts`
                    : "—"}
                </span>
                <ChevronRight className="h-4 w-4 text-slate-400" />
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
