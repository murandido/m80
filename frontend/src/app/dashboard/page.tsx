import type { Metadata } from "next"
import { cookies } from "next/headers"
import { AlertTriangle } from "lucide-react"
import { z } from "zod"
import { env } from "@/lib/env"
import { canAccess } from "@/lib/navigation"
import { getSessionUser } from "@/lib/session"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { getDashboardSchema, type Dashboard } from "@/schemas/dashboard"
import { getAgenciesSchema, type Agency } from "@/schemas/agency"
import { getLoyaltyProgramsSchema, type LoyaltyProgram } from "@/schemas/loyalty-program"
import { getAirlinesSchema, type Airline } from "@/schemas/airline"
import { getUsersSchema, type User } from "@/schemas/user"
import { PeriodFilter } from "@/components/filters/period-filter"
import { LiveRefresh } from "@/components/layout/live-refresh"
import { DashboardFilterBar } from "@/components/dashboard/dashboard-filter-bar"
import { DashboardStats } from "@/components/dashboard/dashboard-stats"
import { MetricToggle } from "@/components/dashboard/metric-toggle"
import { IssuancesOverTimeChart } from "@/components/dashboard/issuances-over-time-chart"
import { PointsByProgramChart } from "@/components/dashboard/points-by-program-chart"
import { PointsByIssuerChart } from "@/components/dashboard/points-by-issuer-chart"
import {
  buildDashboardQuery,
  granularityFor,
  resolveDashboardFilters,
  resolveDashboardMetric,
  resolveDashboardPeriod,
  DASHBOARD_URL_KEYS,
  type DashboardSearchParams,
} from "./dashboard-params"

export const metadata: Metadata = {
  title: "Dashboard",
  description:
    "O resumo do período: quanto foi emitido, quanto entrou e o que ainda está em aberto.",
}

// Returns null when the request or the parse failed - distinct from an empty result, which
// is a legitimate answer. Mirrors fetchList in app/dashboard/sales/page.tsx.
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

async function fetchDashboard(
  url: string,
  authHeader: Record<string, string>,
): Promise<Dashboard | null> {
  try {
    const res = await fetch(url, { headers: authHeader, cache: "no-store" })
    const parsed = getDashboardSchema.response.safeParse(await res.json())
    if (!res.ok || !parsed.success) return null
    return parsed.data.data ?? null
  } catch {
    return null
  }
}

export default async function DashboardPage({
  searchParams,
}: {
  searchParams: Promise<DashboardSearchParams>
}) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  // A bare URL (every sidebar link is one) falls back to the filters this browser
  // session last used here, so the screen is already filtered on the first paint.
  const params = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard"),
    DASHBOARD_URL_KEYS,
  )
  const period = resolveDashboardPeriod(params)
  const filters = resolveDashboardFilters(params)
  const query = buildDashboardQuery(period, filters)

  const [user, dashboard, agencies, loyaltyPrograms, airlines, users] = await Promise.all([
    getSessionUser(),
    fetchDashboard(`${api}/dashboard?${query.toString()}`, authHeader),
    fetchList<Agency>(`${api}/agencies`, getAgenciesSchema.response, authHeader),
    fetchList<LoyaltyProgram>(
      `${api}/loyalty-programs`,
      getLoyaltyProgramsSchema.response,
      authHeader,
    ),
    fetchList<Airline>(`${api}/airlines`, getAirlinesSchema.response, authHeader),
    fetchList<User>(`${api}/users`, getUsersSchema.response, authHeader),
  ])

  // Money follows the same rule the backend applies in dashboardIncludesAmount.
  const canSeeAmounts = canAccess(user?.role, ["admin"])
  const metric = resolveDashboardMetric(params, canSeeAmounts)

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
        <h1 className="text-lg font-semibold text-slate-800">Dashboard</h1>

        <DashboardFilterBar
          agencies={agencies ?? []}
          loyaltyPrograms={loyaltyPrograms ?? []}
          airlines={airlines ?? []}
          users={users ?? []}
          initialFilters={effectiveFilters}
        >
          <div className="flex flex-wrap items-end gap-3">
            <PeriodFilter
              dateFrom={period.date_from}
              dateTo={period.date_to}
              preset={period.preset}
              reference={period.reference}
            />
            <MetricToggle metric={metric} canSeeAmounts={canSeeAmounts} />
          </div>
        </DashboardFilterBar>

        {dashboard === null ? (
          <Alert variant="destructive">
            <AlertTriangle />
            <AlertTitle>Não foi possível carregar o dashboard</AlertTitle>
            <AlertDescription>
              Recarregue a página. Os números podem estar incompletos e não devem ser
              usados para fechar o período.
            </AlertDescription>
          </Alert>
        ) : (
          <>
            <DashboardStats dashboard={dashboard} canSeeAmounts={canSeeAmounts} />

            <IssuancesOverTimeChart
              series={dashboard.series}
              metric={metric}
              granularity={granularityFor(period.preset)}
            />

            <div className="grid gap-4 sm:gap-6 lg:grid-cols-2">
              <PointsByProgramChart byProgram={dashboard.by_program} metric={metric} />
              <PointsByIssuerChart byIssuer={dashboard.by_issuer} metric={metric} />
            </div>
          </>
        )}
      </div>
    </div>
  )
}
