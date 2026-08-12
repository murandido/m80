import type { Metadata } from "next"
import { cookies } from "next/headers"
import Link from "next/link"
import { AlertTriangle, ExternalLink } from "lucide-react"
import { z } from "zod"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"
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
  ALL_AGENCIES,
  buildSalesQuery,
  resolveSalesFilters,
  resolveSalesPeriod,
  SALES_URL_KEYS,
  type SalesSearchParams,
} from "../period-params"

export const metadata: Metadata = {
  title: "Fornecedores",
  description:
    "Quantas milhas saíram de cada conta no período e quanto a agência deve a cada administrador.",
}

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

  // A bare URL (every sidebar link is one) falls back to the filters this browser
  // session last used here, so the screen is already filtered on the first paint.
  const params = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/sales/suppliers"),
    SALES_URL_KEYS,
  )
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

  // The multi-picker owns the agency choice here, so it has to be told what the server
  // actually filtered by. A bare URL means the backend fell back to the main agency; the
  // picker shows that one selected rather than the "todas" it would show for an empty list,
  // which would claim the table holds every agency when it does not.
  const effectiveFilters = {
    ...filters,
    agency_ids:
      filters.agency_ids ||
      filters.agency_id ||
      (agencies ?? []).find((a) => a.is_default)?.id ||
      ALL_AGENCIES,
  }

  return (
    <div className="p-6 sm:p-8">
      <LiveRefresh />
      <div className="flex flex-col gap-4 sm:gap-6">
        <div className="flex items-center justify-between gap-4">
          <h1 className="text-lg font-semibold text-slate-800">Controle de pagamentos</h1>
          <Button asChild variant="ghost" size="sm" className="text-slate-500">
            <Link href="/dashboard/clients">
              Gerenciar clientes
              <ExternalLink className="h-4 w-4" />
            </Link>
          </Button>
        </div>

        <SalesViewToggle active="suppliers" canSeeSuppliers />

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
          <SalesSuppliersTable balances={balances} query={query.toString()} />
        )}
      </div>
    </div>
  )
}
