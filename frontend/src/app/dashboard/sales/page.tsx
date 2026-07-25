import { cookies } from "next/headers"
import Link from "next/link"
import { AlertTriangle, ExternalLink } from "lucide-react"
import { z } from "zod"
import { env } from "@/lib/env"
import { getSessionUser } from "@/actions/auth"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { getSalesSchema, type Sale } from "@/schemas/sale"
import { getClientsSchema, type Client } from "@/schemas/client"
import { getAgenciesSchema, type Agency } from "@/schemas/agency"
import { getLoyaltyProgramsSchema, type LoyaltyProgram } from "@/schemas/loyalty-program"
import { getAccountsSchema, type Account } from "@/schemas/account"
import { getPeopleSchema, type Person } from "@/schemas/person"
import { getUsersSchema, type User } from "@/schemas/user"
import { SalesViewToggle } from "@/components/sales/sales-view-toggle"
import { SalesPeriodFilter } from "@/components/sales/sales-period-filter"
import { SalesFilterBar } from "@/components/sales/sales-filter-bar"
import { SalesClientsTable } from "@/components/sales/sales-clients-table"
import { SalesLiveRefresh } from "@/components/sales/sales-live-refresh"
import {
  buildSalesQuery,
  resolveSalesFilters,
  resolveSalesPeriod,
  type SalesSearchParams,
} from "./period-params"

// Returns null when the request or the parse failed - distinct from an empty list, which
// is a legitimate result. Mirrors fetchList in app/dashboard/issuances/page.tsx.
async function fetchList<T>(
  url: string,
  schema: z.ZodType<{ data?: T[] | null | undefined }>,
  authHeader: Record<string, string>,
): Promise<T[] | null> {
  try {
    const res = await fetch(url, { headers: authHeader, cache: "no-store" })
    const parsed = schema.safeParse(await res.json())
    if (!res.ok || !parsed.success) return null
    return parsed.data.data ?? []
  } catch {
    return null
  }
}

export default async function SalesClientsPage({
  searchParams,
}: {
  searchParams: Promise<SalesSearchParams>
}) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  const params = await searchParams
  const period = resolveSalesPeriod(params)
  const filters = resolveSalesFilters(params)
  const query = buildSalesQuery(period, filters)

  const [user, sales, clients, agencies, loyaltyPrograms, accounts, people, users] =
    await Promise.all([
      getSessionUser(),
      fetchList<Sale>(`${api}/sales?${query.toString()}`, getSalesSchema.response, authHeader),
      fetchList<Client>(`${api}/clients`, getClientsSchema.response, authHeader),
      fetchList<Agency>(`${api}/agencies`, getAgenciesSchema.response, authHeader),
      fetchList<LoyaltyProgram>(
        `${api}/loyalty-programs`,
        getLoyaltyProgramsSchema.response,
        authHeader,
      ),
      fetchList<Account>(`${api}/accounts`, getAccountsSchema.response, authHeader),
      fetchList<Person>(`${api}/people`, getPeopleSchema.response, authHeader),
      fetchList<User>(`${api}/users`, getUsersSchema.response, authHeader),
    ])

  // The backend falls back to the main agency when agency_id is absent; resolving it here
  // too is what lets the combobox show its name instead of an empty placeholder.
  const effectiveFilters = {
    ...filters,
    agency_id: filters.agency_id || (agencies ?? []).find((a) => a.is_default)?.id || "",
  }

  return (
    <div className="p-6 sm:p-8">
      <SalesLiveRefresh />
      <div className="flex flex-col gap-4 sm:gap-6">
        <div className="flex items-center justify-between gap-4">
          <h1 className="text-lg font-semibold text-slate-800">Vendas</h1>
          {/* Linking a sale to a client that does not exist yet is the common case here,
              so the client register is one click away instead of a trip to the sidebar. */}
          <Button asChild variant="ghost" size="sm" className="text-slate-500">
            <Link href="/dashboard/clients">
              Gerenciar clientes
              <ExternalLink className="h-4 w-4" />
            </Link>
          </Button>
        </div>

        <SalesViewToggle active="clients" isAdmin={user?.role === "admin"} />

        <SalesFilterBar
          variant="clients"
          agencies={agencies ?? []}
          loyaltyPrograms={loyaltyPrograms ?? []}
          accounts={accounts ?? []}
          people={people ?? []}
          users={users ?? []}
          clients={clients ?? []}
          initialFilters={effectiveFilters}
        >
          <SalesPeriodFilter
            dateFrom={period.date_from}
            dateTo={period.date_to}
            preset={period.preset}
            reference={period.reference}
          />
        </SalesFilterBar>

        {sales === null ? (
          <Alert variant="destructive">
            <AlertTriangle />
            <AlertTitle>Não foi possível carregar as vendas</AlertTitle>
            <AlertDescription>
              Recarregue a página. Marcar pagamentos com a lista incompleta pode registrar
              uma cobrança na venda errada.
            </AlertDescription>
          </Alert>
        ) : (
          <SalesClientsTable sales={sales} clients={clients ?? []} />
        )}
      </div>
    </div>
  )
}
