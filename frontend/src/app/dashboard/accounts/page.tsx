import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getAccountsSchema, Account } from "@/schemas/account"
import { getPeopleSchema, Person } from "@/schemas/person"
import { getLoyaltyProgramsSchema, LoyaltyProgram } from "@/schemas/loyalty-program"
import { ContasPessoasToggle } from "@/components/layout/contas-pessoas-toggle"
import { AccountCard } from "@/components/accounts/account-card"
import { CreateAccountForm } from "@/components/accounts/create-account-form"

export default async function AccountsPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let accounts: Account[] = []
  let people: Person[] = []
  let loyaltyPrograms: LoyaltyProgram[] = []

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/accounts`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getAccountsSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      accounts = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getPeopleSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      people = parsed.data.data
    }
  } catch {
    // silently fail — combobox/title lookups just come up empty
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/loyalty-programs`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getLoyaltyProgramsSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      loyaltyPrograms = parsed.data.data
    }
  } catch {
    // silently fail — combobox just comes up empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Contas e Pessoas
        </h1>

        <ContasPessoasToggle active="contas" />

        <CreateAccountForm people={people} loyaltyPrograms={loyaltyPrograms} />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {accounts.map((account) => (
            <AccountCard
              key={account.id}
              account={account}
              people={people}
              loyaltyPrograms={loyaltyPrograms}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
