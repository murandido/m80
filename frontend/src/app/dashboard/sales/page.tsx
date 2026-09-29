import type { Metadata } from "next"
import { cookies } from "next/headers"
import Link from "next/link"
import { AlertTriangle, ExternalLink, FileText } from "lucide-react"
import { env } from "@/lib/env"
import { fetchList } from "@/lib/fetch-list"
import { canAccess } from "@/lib/navigation"
import { getSessionUser } from "@/lib/session"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { getClientPaymentsSchema, getSalesSchema, type Sale } from "@/schemas/sale"
import { getClientsSchema, type Client } from "@/schemas/client"
import { getAgenciesSchema, type Agency } from "@/schemas/agency"
import { SalesViewToggle } from "@/components/sales/sales-view-toggle"
import { SalesFilterBar } from "@/components/sales/sales-filter-bar"
import { SalesPeriodFilter } from "@/components/sales/sales-period-filter"
import { SalesClientsTable } from "@/components/sales/sales-clients-table"
import { SalesHelpPopover } from "@/components/sales/sales-help-popover"
import { ClientReportButton } from "@/components/sales/client-report-button"
import { LiveRefresh } from "@/components/layout/live-refresh"
import {
  buildSalesQuery,
  resolveSalesFilters,
  resolveSalesPeriod,
  SALES_URL_KEYS,
  singleClientId,
  type SalesSearchParams,
} from "./period-params"
import { defaultAgencyId } from "@/lib/default-agency"

export const metadata: Metadata = {
  title: "Controle de pagamentos",
  description:
    "Quanto cada cliente comprou no período e quanto ainda deve à agência.",
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
  // This view only offers agency, client and payment status. Program, account and issuer
  // are dropped even when the URL or the remembered cookie still carries them (the screen
  // used to have those filters): applied with no control on screen, they would hide sales
  // with nothing saying why.
  const filters = {
    ...resolveSalesFilters(params),
    loyalty_program_id: "",
    account_id: "",
    issuer_id: "",
  }
  const query = buildSalesQuery(period, filters)

  // One client filtered: the table closes with what that client owes, and their credit is
  // part of the answer - money already handed over that no sale has taken yet.
  const filteredClientId = singleClientId(filters.client_ids)

  const [user, sales, clients, agencies, clientCreditCents] = await Promise.all([
    getSessionUser(),
    fetchList<Sale>(
      `${api}/sales?${query.toString()}`,
      getSalesSchema.response,
      authHeader,
    ),
    fetchList<Client>(`${api}/clients`, getClientsSchema.response, authHeader),
    fetchList<Agency>(
      `${api}/agencies`,
      getAgenciesSchema.response,
      authHeader,
    ),
    filteredClientId ? fetchClientCredit(api, filteredClientId, authHeader) : null,
  ])

  // The backend falls back to the user's default agency when agency_id is absent; resolving
  // it here too is what lets the combobox show its name instead of an empty placeholder.
  const effectiveFilters = {
    ...filters,
    agency_id: filters.agency_id || defaultAgencyId(agencies) || "",
  }

  return (
    <div className="p-4 sm:p-8">
      <LiveRefresh />
      <div className="flex flex-col gap-4 sm:gap-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-lg font-semibold text-slate-800">
            Controle de pagamentos
          </h1>
          <div className="flex flex-wrap items-center gap-2">
            {/* Linking a sale to a client that does not exist yet is the common case here,
                so the client register is one click away instead of a trip to the sidebar. */}
            <Button asChild variant="ghost" className="text-slate-500">
              <Link href="/dashboard/clients">
                Gerenciar clientes
                <ExternalLink className="h-4 w-4" />
              </Link>
            </Button>
            <Button asChild variant="outline">
              {/* Native download: the backend sets the filename via Content-Disposition, and
                  `query` is the same string that fetched the list on screen. */}
              <a href={`/api/sales/export?${query.toString()}`} download>
                <FileText className="h-4 w-4" />
                Exportar
              </a>
            </Button>
            <ClientReportButton query={query.toString()} />
            <SalesHelpPopover />
          </div>
        </div>

        <SalesViewToggle
          active="clients"
          canSeeSuppliers={canAccess(user?.role, ["admin"])}
        />

        <SalesFilterBar
          variant="clients"
          agencies={agencies ?? []}
          loyaltyPrograms={[]}
          accounts={[]}
          people={[]}
          initialFilters={effectiveFilters}
        >
          <SalesPeriodFilter dateFrom={period.date_from} dateTo={period.date_to} />
        </SalesFilterBar>

        {sales === null ? (
          <Alert variant="destructive">
            <AlertTriangle />
            <AlertTitle>Não foi possível carregar as vendas</AlertTitle>
            <AlertDescription>
              Recarregue a página. Marcar pagamentos com a lista incompleta pode
              registrar uma cobrança na venda errada.
            </AlertDescription>
          </Alert>
        ) : (
          <SalesClientsTable
            sales={sales}
            // A sale's client belongs to the sale's agency, so the pickers offer only the
            // clients of the agency this list is showing.
            clients={(clients ?? []).filter((c) => c.agency_id === effectiveFilters.agency_id)}
            agencyId={effectiveFilters.agency_id}
            clientFilter={filters.client_ids}
            clientCreditCents={clientCreditCents}
            dateFrom={period.date_from}
            dateTo={period.date_to}
            paidFilter={filters.paid}
          />
        )}
      </div>
    </div>
  )
}

// The client's credit, or null when it could not be read - the total row then shows the
// debt alone rather than a credit of zero that nobody checked.
async function fetchClientCredit(
  api: string,
  clientId: string,
  authHeader: Record<string, string>,
): Promise<number | null> {
  try {
    const res = await fetch(
      `${api}/sales/client-payments?client_id=${encodeURIComponent(clientId)}`,
      { headers: authHeader, cache: "no-store" },
    )
    const parsed = getClientPaymentsSchema.response.safeParse(await res.json())
    if (!res.ok || !parsed.success) return null
    return parsed.data.data?.credit_cents ?? null
  } catch {
    return null
  }
}
