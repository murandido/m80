import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { getIssuancesSchema, Issuance } from "@/schemas/issuance"
import { getLoyaltyProgramsSchema, LoyaltyProgram } from "@/schemas/loyalty-program"
import { getAirlinesSchema, Airline } from "@/schemas/airline"
import { getAccountsSchema, Account } from "@/schemas/account"
import { getAgenciesSchema, Agency } from "@/schemas/agency"
import { getUsersSchema, getMeSchema, User } from "@/schemas/user"
import { getPeopleSchema, Person } from "@/schemas/person"
import { IssuancesGrid } from "@/components/issuances/issuances-grid"

export default async function IssuancesPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let issuances: Issuance[] = []
  let loyaltyPrograms: LoyaltyProgram[] = []
  let airlines: Airline[] = []
  let accounts: Account[] = []
  let agencies: Agency[] = []
  let users: User[] = []
  let people: Person[] = []
  let currentUserId: string | null = null

  try {
    const [issuancesRes, loyaltyProgramsRes, airlinesRes, accountsRes, agenciesRes, usersRes, peopleRes, meRes] =
      await Promise.all([
        fetch(`${env.BACKEND_URL}/api/v1/issuances`, { headers: authHeader }),
        fetch(`${env.BACKEND_URL}/api/v1/loyalty-programs`, { headers: authHeader }),
        fetch(`${env.BACKEND_URL}/api/v1/airlines`, { headers: authHeader }),
        fetch(`${env.BACKEND_URL}/api/v1/accounts`, { headers: authHeader }),
        fetch(`${env.BACKEND_URL}/api/v1/agencies`, { headers: authHeader }),
        fetch(`${env.BACKEND_URL}/api/v1/users`, { headers: authHeader }),
        fetch(`${env.BACKEND_URL}/api/v1/people`, { headers: authHeader }),
        fetch(`${env.BACKEND_URL}/api/v1/auth/me`, { headers: authHeader }),
      ])

    const [issuancesJson, loyaltyProgramsJson, airlinesJson, accountsJson, agenciesJson, usersJson, peopleJson, meJson] =
      await Promise.all([
        issuancesRes.json(),
        loyaltyProgramsRes.json(),
        airlinesRes.json(),
        accountsRes.json(),
        agenciesRes.json(),
        usersRes.json(),
        peopleRes.json(),
        meRes.json(),
      ])

    const parsedIssuances = getIssuancesSchema.response.safeParse(issuancesJson)
    if (parsedIssuances.success && parsedIssuances.data.data) issuances = parsedIssuances.data.data

    const parsedLoyaltyPrograms = getLoyaltyProgramsSchema.response.safeParse(loyaltyProgramsJson)
    if (parsedLoyaltyPrograms.success && parsedLoyaltyPrograms.data.data) {
      loyaltyPrograms = parsedLoyaltyPrograms.data.data
    }

    const parsedAirlines = getAirlinesSchema.response.safeParse(airlinesJson)
    if (parsedAirlines.success && parsedAirlines.data.data) airlines = parsedAirlines.data.data

    const parsedAccounts = getAccountsSchema.response.safeParse(accountsJson)
    if (parsedAccounts.success && parsedAccounts.data.data) accounts = parsedAccounts.data.data

    const parsedAgencies = getAgenciesSchema.response.safeParse(agenciesJson)
    if (parsedAgencies.success && parsedAgencies.data.data) agencies = parsedAgencies.data.data

    const parsedUsers = getUsersSchema.response.safeParse(usersJson)
    if (parsedUsers.success && parsedUsers.data.data) users = parsedUsers.data.data

    const parsedPeople = getPeopleSchema.response.safeParse(peopleJson)
    if (parsedPeople.success && parsedPeople.data.data) people = parsedPeople.data.data

    const parsedMe = getMeSchema.response.safeParse(meJson)
    if (parsedMe.success && parsedMe.data.data) currentUserId = parsedMe.data.data.id
  } catch {
    // silently fail — grid stays empty, save actions will surface errors individually
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Emissões</h1>

        <IssuancesGrid
          initialIssuances={issuances}
          loyaltyPrograms={loyaltyPrograms}
          airlines={airlines}
          accounts={accounts}
          agencies={agencies}
          users={users}
          people={people}
          currentUserId={currentUserId}
        />
      </div>
    </div>
  )
}
