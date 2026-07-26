import type { PeriodPreset } from "@/lib/date-range"
import {
  type PeriodSearchParams,
  type ResolvedPeriod,
  resolvePeriod,
} from "@/lib/period-params"

// The dimensions the dashboard can slice by. Deliberately narrower than the sales filters:
// client and paid are properties of a sale, not of the volume this page measures.
export const DASHBOARD_FILTER_KEYS = [
  "agency_id",
  "loyalty_program_id",
  "airline_id",
  "issuer_id",
] as const

export type DashboardFilterState = Record<
  (typeof DASHBOARD_FILTER_KEYS)[number],
  string
>

// Which unit the charts are drawn in. Only admins can select "amount"; the server decides
// whether to send the numbers at all, so a hand-typed metric=amount changes nothing for an
// operator beyond an axis label with no data behind it.
export const DASHBOARD_METRICS = ["points", "amount"] as const

export type DashboardMetric = (typeof DASHBOARD_METRICS)[number]

export type DashboardSearchParams = PeriodSearchParams &
  Partial<Record<(typeof DASHBOARD_FILTER_KEYS)[number] | "metric", string>>

// Defaults to today: the dashboard's job is "how is the agency doing right now", and the
// operator can widen to the month in one click.
export function resolveDashboardPeriod(params: DashboardSearchParams): ResolvedPeriod {
  return resolvePeriod(params, "day")
}

export function resolveDashboardFilters(
  params: DashboardSearchParams,
): DashboardFilterState {
  return {
    agency_id: params.agency_id ?? "",
    loyalty_program_id: params.loyalty_program_id ?? "",
    airline_id: params.airline_id ?? "",
    issuer_id: params.issuer_id ?? "",
  }
}

export function resolveDashboardMetric(
  params: DashboardSearchParams,
  isAdmin: boolean,
): DashboardMetric {
  if (!isAdmin) return "points"
  return params.metric === "amount" ? "amount" : "points"
}

// How wide a bucket the time series uses, derived from the period rather than exposed as a
// second control: a year split by day is 365 unreadable columns, and a single day split by
// day is one. One rule, in one place, so the axis always matches the window.
export function granularityFor(preset: PeriodPreset): "hour" | "day" | "month" {
  switch (preset) {
    case "day":
      return "hour"
    case "year":
      return "month"
    default:
      return "day"
  }
}

export function buildDashboardQuery(
  period: ResolvedPeriod,
  filters: DashboardFilterState,
): URLSearchParams {
  const query = new URLSearchParams({
    date_from: period.date_from,
    date_to: period.date_to,
    granularity: granularityFor(period.preset),
  })
  for (const key of DASHBOARD_FILTER_KEYS) {
    if (filters[key]) query.set(key, filters[key])
  }
  return query
}
