import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft } from "lucide-react"
import { Button } from "@/components/ui/button"
import { getPortalAccountsAction, getPortalEntriesAction } from "@/actions/portal"
import { AccountStatementView } from "@/components/accounts/account-statement-view"
import { PortalConnectPanel } from "@/components/portal/portal-connect-panel"
import type { SyncedProgram } from "@/schemas/inventory"

export const metadata: Metadata = {
  title: "Extrato · M80",
  description: "O extrato sincronizado da sua conta.",
}

export default async function PortalAccountPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = await params

  // The account comes from the client's own list, which is the grant: an id that is not there
  // is someone else's account (or none), and the API would answer 404 for it anyway.
  const accountsResult = await getPortalAccountsAction()
  const account =
    "error" in accountsResult ? null : accountsResult.accounts.find((item) => item.id === id)
  if (!account) {
    notFound()
  }

  const syncedProgram: SyncedProgram | null =
    account.program_code === "AZUL" ? "azul" : account.program_code === "SMILES" ? "smiles" : null
  if (!syncedProgram) {
    notFound()
  }

  const result = await getPortalEntriesAction(id)

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 px-4 py-6">
      <div>
        <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2 text-slate-500">
          <Link href="/portal">
            <ArrowLeft className="mr-1 h-4 w-4" />
            Suas contas
          </Link>
        </Button>
      </div>

      <PortalConnectPanel
        accountId={id}
        program={syncedProgram}
        connected={account.connected}
        state={account.state}
        hasPassword={account.has_password}
      />

      {"error" in result ? (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
          {result.error}
        </div>
      ) : (
        <AccountStatementView
          variant="client"
          accountId={id}
          accountName={account.holder_name}
          accountDocument={account.login ?? null}
          program={{
            id: account.program_id,
            name: account.program_name,
            code: account.program_code,
            color: account.program_color,
          }}
          syncedProgram={syncedProgram}
          entries={result.entries}
        />
      )}
    </div>
  )
}
