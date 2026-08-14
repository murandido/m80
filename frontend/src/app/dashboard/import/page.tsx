import type { Metadata } from "next"
import { cookies } from "next/headers"
import { z } from "zod"
import { env } from "@/lib/env"
import { getAccountsSchema } from "@/schemas/account"
import { getAgenciesSchema } from "@/schemas/agency"
import { getPeopleSchema } from "@/schemas/person"
import { getUsersSchema } from "@/schemas/user"
import { LegacyImportScreen } from "@/components/legacy-import/legacy-import-screen"

export const metadata: Metadata = {
  title: "Importar planilha",
  description:
    "Traga para o painel um dia da planilha de controle de emissões feita à mão.",
}

// Temporary screen: it exists to migrate the team off the hand-kept spreadsheet and is
// deleted with the rest of the importer once that is done. See
// backend/internal/legacyimport for the full list of what goes.

// Returns null when the request or the parse failed, which is different from an empty
// list - the same convention the issuances page uses.
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

export default async function LegacyImportPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  // What the mapping controls need: accounts to point an alias at (with the people that
  // give an account its name), users to credit a block to, agencies to charge the day to.
  const [accounts, people, users, agencies] = await Promise.all([
    fetchList(`${api}/accounts`, getAccountsSchema.response, authHeader),
    fetchList(`${api}/people`, getPeopleSchema.response, authHeader),
    fetchList(`${api}/users`, getUsersSchema.response, authHeader),
    fetchList(`${api}/agencies`, getAgenciesSchema.response, authHeader),
  ])

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <div>
          <h1 className="text-lg font-semibold text-slate-800">Importar planilha</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Traga um dia da planilha de controle de emissões para o painel. Nada é gravado
            até você conferir a prévia e clicar em importar.
          </p>
        </div>

        <LegacyImportScreen
          accounts={accounts ?? []}
          people={people ?? []}
          users={users ?? []}
          agencies={agencies ?? []}
        />
      </div>
    </div>
  )
}
