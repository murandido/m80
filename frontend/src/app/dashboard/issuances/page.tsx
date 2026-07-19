import { cookies } from "next/headers"
import { AlertTriangle } from "lucide-react"
import { z } from "zod"
import { env } from "@/lib/env"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { getIssuancesSchema } from "@/schemas/issuance"
import { getLoyaltyProgramsSchema } from "@/schemas/loyalty-program"
import { getAirlinesSchema } from "@/schemas/airline"
import { getAccountsSchema } from "@/schemas/account"
import { getAgenciesSchema } from "@/schemas/agency"
import { getUsersSchema, getMeSchema } from "@/schemas/user"
import { getPeopleSchema } from "@/schemas/person"
import { IssuancesGrid } from "@/components/issuances/issuances-grid"

// Returns null when the request or the parse failed - distinct from an empty list,
// which is a legitimate result. Each call swallows its own error so one dead endpoint
// can't blank out the whole screen, and so the caller can tell the two apart.
async function fetchList<T>(
  url: string,
  schema: z.ZodType<{ data?: T[] | null | undefined }>,
  authHeader: Record<string, string>,
): Promise<T[] | null> {
  try {
    const res = await fetch(url, { headers: authHeader })
    const parsed = schema.safeParse(await res.json())
    if (!res.ok || !parsed.success) return null
    return parsed.data.data ?? []
  } catch {
    return null
  }
}

async function fetchCurrentUserId(authHeader: Record<string, string>): Promise<string | null> {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/auth/me`, { headers: authHeader })
    const parsed = getMeSchema.response.safeParse(await res.json())
    if (!res.ok || !parsed.success || !parsed.data.data) return null
    return parsed.data.data.id
  } catch {
    return null
  }
}

export default async function IssuancesPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  // Every promise below resolves (never rejects), so a single failure degrades only
  // its own list instead of taking the other seven down with it.
  const [issuances, loyaltyPrograms, airlines, accounts, agencies, users, people, currentUserId] =
    await Promise.all([
      fetchList(`${api}/issuances`, getIssuancesSchema.response, authHeader),
      fetchList(`${api}/loyalty-programs`, getLoyaltyProgramsSchema.response, authHeader),
      fetchList(`${api}/airlines`, getAirlinesSchema.response, authHeader),
      fetchList(`${api}/accounts`, getAccountsSchema.response, authHeader),
      fetchList(`${api}/agencies`, getAgenciesSchema.response, authHeader),
      fetchList(`${api}/users`, getUsersSchema.response, authHeader),
      fetchList(`${api}/people`, getPeopleSchema.response, authHeader),
      fetchCurrentUserId(authHeader),
    ])

  const referenceDataFailed = [loyaltyPrograms, airlines, accounts, agencies, users, people].some(
    (list) => list === null,
  )

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Emissões</h1>

        {issuances === null ? (
          // The grid is deliberately not rendered: showing an empty spreadsheet after a
          // failed load invites the user to retype issuances that already exist.
          <Alert variant="destructive">
            <AlertTriangle />
            <AlertTitle>Não foi possível carregar as emissões</AlertTitle>
            <AlertDescription>
              As emissões já cadastradas não puderam ser lidas do servidor. Recarregue a página —
              cadastrar agora poderia duplicar emissões existentes.
            </AlertDescription>
          </Alert>
        ) : (
          <>
            {referenceDataFailed && (
              <Alert>
                <AlertTriangle />
                <AlertTitle>Algumas listas não carregaram</AlertTitle>
                <AlertDescription>
                  Companhias, contas, agências ou emissores podem aparecer incompletos nos campos de
                  seleção. Recarregue a página para tentar de novo.
                </AlertDescription>
              </Alert>
            )}

            <IssuancesGrid
              initialIssuances={issuances}
              loyaltyPrograms={loyaltyPrograms ?? []}
              airlines={airlines ?? []}
              accounts={accounts ?? []}
              agencies={agencies ?? []}
              users={users ?? []}
              people={people ?? []}
              currentUserId={currentUserId}
            />
          </>
        )}
      </div>
    </div>
  )
}
