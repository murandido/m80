import {
  PERIOD_PRESETS,
  type PeriodPreset,
  formatYmd,
  parseYmd,
  resolvePeriodRange,
  startOfToday,
} from "@/lib/date-range"

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

// `period`/`reference` are what the user picked; date_from/date_to are the expansion the
// API actually filters on, kept in the URL so a shared link reproduces the exact range.
export type SalesSearchParams = Partial<
  Record<(typeof SALES_FILTER_KEYS)[number] | "period" | "reference" | "date_from" | "date_to", string>
>

export interface ResolvedPeriod {
  preset: PeriodPreset
  reference: string
  date_from: string
  date_to: string
}

function isPeriodPreset(value: string | undefined): value is PeriodPreset {
  return PERIOD_PRESETS.includes(value as PeriodPreset)
}

// Defaults to the current month: a single day is too narrow to be useful as a landing
// view for "who still owes me money".
export function resolveSalesPeriod(params: SalesSearchParams): ResolvedPeriod {
  const preset = isPeriodPreset(params.period) ? params.period : "month"
  const referenceDate = parseYmd(params.reference ?? "") ?? startOfToday()
  const range = resolvePeriodRange(preset, referenceDate)

  return {
    preset,
    reference: formatYmd(referenceDate),
    // An explicit date_from/date_to in the URL wins, so a hand-edited range survives a
    // reload instead of snapping back to the preset's expansion.
    date_from: params.date_from || range.date_from,
    date_to: params.date_to || range.date_to,
  }
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
