import {
  PERIOD_KEYS,
  type PeriodSearchParams,
  type ResolvedPeriod,
  resolvePeriod,
} from "@/lib/period-params"

export type { ResolvedPeriod }

// The dimensions both sales views can filter on, carried as URL query params. Empty string
// means "not filtered" on that dimension - except agency_id, where empty means "the main
// agency", resolved by the backend.
export const SALES_FILTER_KEYS = [
  "agency_id",
  "loyalty_program_id",
  "account_id",
  "issuer_id",
  "client_id",
  "paid",
] as const

export type SalesFilterState = Record<(typeof SALES_FILTER_KEYS)[number], string>

export type SalesSearchParams = PeriodSearchParams &
  Partial<Record<(typeof SALES_FILTER_KEYS)[number], string>>

// Everything the sales views own in the URL - what has to be remembered, and what
// tells the page that the URL is user-controlled rather than a bare sidebar link.
export const SALES_URL_KEYS = [...PERIOD_KEYS, ...SALES_FILTER_KEYS] as const

// Defaults to the current month: a single day is too narrow to be useful as a landing
// view for "who still owes me money".
export function resolveSalesPeriod(params: SalesSearchParams): ResolvedPeriod {
  return resolvePeriod(params, "month")
}

export function resolveSalesFilters(params: SalesSearchParams): SalesFilterState {
  return {
    agency_id: params.agency_id ?? "",
    loyalty_program_id: params.loyalty_program_id ?? "",
    account_id: params.account_id ?? "",
    issuer_id: params.issuer_id ?? "",
    client_id: params.client_id ?? "",
    paid: params.paid ?? "",
  }
}

// Builds the query string for the backend out of the resolved period plus filters. Empty
// dimensions are left out entirely rather than sent as "", which the Go handlers read as
// "no filter" anyway - this just keeps the request URL readable.
export function buildSalesQuery(
  period: ResolvedPeriod,
  filters: SalesFilterState,
): URLSearchParams {
  const query = new URLSearchParams({
    date_from: period.date_from,
    date_to: period.date_to,
  })
  for (const key of SALES_FILTER_KEYS) {
    if (filters[key]) query.set(key, filters[key])
  }
  return query
}
