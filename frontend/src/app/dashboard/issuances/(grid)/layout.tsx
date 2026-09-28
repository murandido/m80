import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { fetchList } from "@/lib/fetch-list"
import { getLoyaltyProgramsSchema } from "@/schemas/loyalty-program"
import { getAirlinesSchema } from "@/schemas/airline"
import { getAccountOptionsSchema } from "@/schemas/account"
import { getAgenciesSchema } from "@/schemas/agency"
import { getUsersSchema } from "@/schemas/user"
import { getPeopleOptionsSchema } from "@/schemas/person"
import { IssuancesReferenceProvider } from "@/components/issuances/issuances-screen"

// The reference lists behind every picker on the issuances grid. They live here, in a
// layout, because a layout is not re-rendered when only the search params change: the
// page below re-renders on every filter change, and fetching these there meant six extra
// backend queries and every account and person serialized again each time - for lists the
// filters do not touch. See IssuancesReferenceProvider.
//
// A route group, so the ticket screen under [id] does not pay for any of it.
//
// The cost of the move: while the operator stays on this screen the lists are as fresh as
// the last full navigation to it. That was already true between filter changes, and the
// grid covers the case that matters - a record created elsewhere reaches it embedded in a
// refetched issuance (see adoptReferences).
export default async function IssuancesReferenceLayout({
  children,
}: {
  children: React.ReactNode
}) {
  const token = (await cookies()).get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  // Each promise resolves (never rejects), so a single failure degrades only its own list.
  const [loyaltyPrograms, airlines, accounts, agencies, users, people] = await Promise.all([
    fetchList(`${api}/loyalty-programs`, getLoyaltyProgramsSchema.response, authHeader),
    fetchList(`${api}/airlines`, getAirlinesSchema.response, authHeader),
    fetchList(`${api}/accounts?view=picker`, getAccountOptionsSchema.response, authHeader),
    fetchList(`${api}/agencies`, getAgenciesSchema.response, authHeader),
    fetchList(`${api}/users`, getUsersSchema.response, authHeader),
    fetchList(`${api}/people?view=picker`, getPeopleOptionsSchema.response, authHeader),
  ])

  const failed = [loyaltyPrograms, airlines, accounts, agencies, users, people].some(
    (list) => list === null,
  )

  return (
    <IssuancesReferenceProvider
      failed={failed}
      data={{
        loyaltyPrograms: loyaltyPrograms ?? [],
        airlines: airlines ?? [],
        accounts: accounts ?? [],
        agencies: agencies ?? [],
        users: users ?? [],
        people: people ?? [],
      }}
    >
      {children}
    </IssuancesReferenceProvider>
  )
}
