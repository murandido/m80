import type { Metadata } from "next"
import { cookies } from "next/headers"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft } from "lucide-react"
import { env } from "@/lib/env"
import { Button } from "@/components/ui/button"
import { requireAdmin } from "@/lib/session"
import { getAccountSchema, type Account } from "@/schemas/account"
import { getAzulStatementEntriesAction } from "@/actions/statement"
import { AccountStatementView } from "@/components/accounts/account-statement-view"

export const metadata: Metadata = {
  title: "Extrato da conta",
  description: "O extrato sincronizado da conta, com cada resgate ligado à emissão real.",
}

async function fetchAccount(id: string): Promise<Account | null> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/accounts/${id}`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) return null
    const parsed = getAccountSchema.response.safeParse(json)
    if (!parsed.success || !parsed.data.data) return null
    return parsed.data.data
  } catch {
    return null
  }
}

// The account's own number has no name; the holder does, and it is what identifies the account
// at a glance. Read separately because the account response carries only the holder's id.
async function fetchHolderName(holderId: string): Promise<string | null> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people/${holderId}`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = (await res.json().catch(() => null)) as { data?: { name?: string } } | null
    if (!res.ok) return null
    return typeof json?.data?.name === "string" ? json.data.name : null
  } catch {
    return null
  }
}

export default async function AccountStatementPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  await requireAdmin()
  const { id } = await params

  const account = await fetchAccount(id)
  if (!account) {
    notFound()
  }

  const result = await getAzulStatementEntriesAction(id)

  const holderName = account.holder_id ? await fetchHolderName(account.holder_id) : null
  // The name leads; the document (the login/CPF) is the secondary line. Fall back to the login as
  // the name for a bot-created account that has no holder yet.
  const accountName =
    holderName ?? account.login ?? account.nicknames[0]?.nickname ?? "Conta sem título"
  const accountDocument = account.login && account.login !== accountName ? account.login : null

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 px-4 py-6">
      <div>
        <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2 text-slate-500">
          <Link href="/dashboard/accounts">
            <ArrowLeft className="mr-1 h-4 w-4" />
            Contas
          </Link>
        </Button>
      </div>

      {"error" in result ? (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
          {result.error}
        </div>
      ) : (
        <AccountStatementView
          accountId={id}
          accountName={accountName}
          accountDocument={accountDocument}
          program={account.loyalty_program}
          entries={result.entries}
        />
      )}
    </div>
  )
}
