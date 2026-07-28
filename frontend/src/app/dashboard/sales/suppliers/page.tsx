import { cookies } from "next/headers"
import Link from "next/link"
import { AlertTriangle, ExternalLink } from "lucide-react"
import { z } from "zod"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { getSupplierBalancesSchema, type SupplierBalance } from "@/schemas/sale"
import { getAgenciesSchema, type Agency } from "@/schemas/agency"
import { getLoyaltyProgramsSchema, type LoyaltyProgram } from "@/schemas/loyalty-program"
import { getAccountsSchema, type Account } from "@/schemas/account"
import { getPeopleSchema, type Person } from "@/schemas/person"
import { SalesViewToggle } from "@/components/sales/sales-view-toggle"
import { PeriodFilter } from "@/components/filters/period-filter"
import { SalesFilterBar } from "@/components/sales/sales-filter-bar"
import { SalesSuppliersTable } from "@/components/sales/sales-suppliers-table"
import { LiveRefresh } from "@/components/layout/live-refresh"
import {
  buildSalesQuery,
  resolveSalesFilters,
  resolveSalesPeriod,
  type SalesSearchParams,
} from "../period-params"

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

export default async function SalesSuppliersPage({
  searchParams,
}: {
  searchParams: Promise<SalesSearchParams>
}) {
  // What the agency owes each account administrator is admin-only. The backend rejects the
  // route with 403 anyway; this just turns that into a redirect instead of an error page.
  await requireAdmin()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  const params = await searchParams
  const period = resolveSalesPeriod(params)
  const filters = resolveSalesFilters(params)
  const query = buildSalesQuery(period, filters)

  const [balances, agencies, loyaltyPrograms, accounts, people] = await Promise.all([
    fetchList<SupplierBalance>(
      `${api}/sales/suppliers?${query.toString()}`,
      getSupplierBalancesSchema.response,
      authHeader,
    ),
    fetchList<Agency>(`${api}/agencies`, getAgenciesSchema.response, authHeader),
    fetchList<LoyaltyProgram>(
      `${api}/loyalty-programs`,
      getLoyaltyProgramsSchema.response,
      authHeader,
    ),
    fetchList<Account>(`${api}/accounts`, getAccountsSchema.response, authHeader),
    fetchList<Person>(`${api}/people`, getPeopleSchema.response, authHeader),
  ])

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
          <Button asChild variant="ghost" size="sm" className="text-slate-500">
            <Link href="/dashboard/clients">
              Gerenciar clientes
              <ExternalLink className="h-4 w-4" />
            </Link>
          </Button>
        </div>

        <SalesViewToggle active="suppliers" isAdmin />

        <SalesFilterBar
          variant="suppliers"
          agencies={agencies ?? []}
          loyaltyPrograms={loyaltyPrograms ?? []}
          accounts={accounts ?? []}
          people={people ?? []}
          users={[]}
          clients={[]}
          initialFilters={effectiveFilters}
        >
          <PeriodFilter
            dateFrom={period.date_from}
            dateTo={period.date_to}
            preset={period.preset}
            reference={period.reference}
          />
        </SalesFilterBar>

        {balances === null ? (
          <Alert variant="destructive">
            <AlertTriangle />
            <AlertTitle>Não foi possível calcular os saldos</AlertTitle>
            <AlertDescription>
              Recarregue a página. Um saldo parcial levaria a acertar com o administrador da
              conta uma quantidade de milhas diferente da devida.
            </AlertDescription>
          </Alert>
        ) : (
          <SalesSuppliersTable balances={balances} />
        )}
      </div>
    </div>
  )
}
