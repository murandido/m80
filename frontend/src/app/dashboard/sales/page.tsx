import type { Metadata } from "next"
import { cookies } from "next/headers"
import Link from "next/link"
import { AlertTriangle, ExternalLink, FileSpreadsheet } from "lucide-react"
import { z } from "zod"
import { env } from "@/lib/env"
import { getSessionUser } from "@/lib/session"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"
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
import { PeriodFilter } from "@/components/filters/period-filter"
import { SalesFilterBar } from "@/components/sales/sales-filter-bar"
import { SalesClientsTable } from "@/components/sales/sales-clients-table"
import { LiveRefresh } from "@/components/layout/live-refresh"
import {
  buildSalesQuery,
  resolveSalesFilters,
  resolveSalesPeriod,
  SALES_URL_KEYS,
  type SalesSearchParams,
} from "./period-params"

export const metadata: Metadata = {
  title: "Vendas",
  description:
    "Quanto cada cliente comprou no período e quanto ainda deve à agência.",
}

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

  // A bare URL (every sidebar link is one) falls back to the filters this browser
  // session last used here, so the screen is already filtered on the first paint.
  const params = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/sales"),
    SALES_URL_KEYS,
  )
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
      <LiveRefresh />
      <div className="flex flex-col gap-4 sm:gap-6">
        <div className="flex items-center justify-between gap-4">
          <h1 className="text-lg font-semibold text-slate-800">Vendas</h1>
          <div className="flex items-center gap-2">
            {/* Linking a sale to a client that does not exist yet is the common case here,
                so the client register is one click away instead of a trip to the sidebar. */}
            <Button asChild variant="ghost" size="sm" className="text-slate-500">
              <Link href="/dashboard/clients">
                Gerenciar clientes
                <ExternalLink className="h-4 w-4" />
              </Link>
            </Button>
            <Button asChild variant="outline" size="sm">
              {/* Native download: the backend sets the filename via Content-Disposition, and
                  `query` is the same string that fetched the list on screen. */}
              <a href={`/api/sales/export?${query.toString()}`} download>
                <FileSpreadsheet className="h-4 w-4" />
                Exportar
              </a>
            </Button>
          </div>
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
          <PeriodFilter
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
