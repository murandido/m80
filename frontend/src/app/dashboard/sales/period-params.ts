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
  // agency_ids is the suppliers view's multi-valued agency choice and takes precedence
  // over agency_id on that screen only. It is a comma-separated list, or the literal
  // "all": an empty value cannot mean "every agency" because the filter bar drops empty
  // params from the URL, which would make "none selected" indistinguishable from "never
  // chose", and the second one still has to mean the main agency.
  "agency_ids",
  "loyalty_program_id",
  "account_id",
  "administrator_id",
  "issuer_id",
  "client_id",
  // client_ids is the clients view's multi-valued client choice, comma-separated, and may
  // carry NO_CLIENT for the sales nobody was linked to.
  "client_ids",
  "paid",
  // group_by is the suppliers view's choice between one row per account and one row per
  // account administrator. It rides with the filters so it reaches the backend, the sticky
  // cookie and the PDF link for free; anything other than "administrator" means account,
  // on both sides.
  "group_by",
] as const

// ALL_AGENCIES is the explicit "every agency" token. It has to match allAgenciesToken in
// backend/internal/api/v1/handlers/sales.go.
export const ALL_AGENCIES = "all"

export type SalesFilterState = Record<(typeof SALES_FILTER_KEYS)[number], string>

export type SalesSearchParams = PeriodSearchParams &
  Partial<Record<(typeof SALES_FILTER_KEYS)[number], string>>

// Everything the sales views own in the URL - what has to be remembered, and what
// tells the page that the URL is user-controlled rather than a bare sidebar link.
export const SALES_URL_KEYS = [...PERIOD_KEYS, ...SALES_FILTER_KEYS] as const

// Defaults to today, matching the dashboard home and the issuances grid. Landing on
// the same day everywhere means a number seen on one screen is comparable with a
// number seen on the next without checking which period each one is showing.
export function resolveSalesPeriod(params: SalesSearchParams): ResolvedPeriod {
  return resolvePeriod(params, "day")
}

// ALL_PERIOD is the clients view's "Todas as emissões": no date filter at all. It travels
// as period=all in the URL - an explicit value, not just missing dates - so the sticky-filter
// cookie can remember it and a bare URL can still mean "the default".
export const ALL_PERIOD = "all"

export type SalesClientsPeriod = ResolvedPeriod & {
  // all says the list is every issuance; date_from/date_to are empty then.
  all: boolean
}

// The clients view defaults to every issuance: it is the list of who still owes what, and
// a debt does not stop being one when the day it was made scrolls out of the filter. The
// suppliers view keeps resolveSalesPeriod and its "today".
export function resolveSalesClientsPeriod(params: SalesSearchParams): SalesClientsPeriod {
  const chosePeriod = Boolean(params.period || params.date_from || params.date_to)
  if (params.period === ALL_PERIOD || !chosePeriod) {
    return { preset: "custom", reference: "", date_from: "", date_to: "", all: true }
  }
  return { ...resolvePeriod(params, "day"), all: false }
}

export function resolveSalesFilters(params: SalesSearchParams): SalesFilterState {
  return {
    agency_id: params.agency_id ?? "",
    agency_ids: params.agency_ids ?? "",
    loyalty_program_id: params.loyalty_program_id ?? "",
    account_id: params.account_id ?? "",
    administrator_id: params.administrator_id ?? "",
    issuer_id: params.issuer_id ?? "",
    // client_id is the single client an older link or a remembered filter cookie still
    // carries. It is read into client_ids and never sent on, so the backend only ever
    // sees one of the two.
    client_id: "",
    client_ids: params.client_ids || params.client_id || "",
    paid: params.paid ?? "",
    group_by: params.group_by ?? "",
  }
}

// NO_CLIENT is the "Sem cliente registrado" entry of the client filter. Has to match
// noClientToken in backend/internal/api/v1/handlers/sales.go.
export const NO_CLIENT = "none"

export function parseClientIds(value: string): string[] {
  return value.split(",").filter(Boolean)
}

export function serializeClientIds(ids: string[]): string {
  return ids.join(",")
}

// singleClientId is the client when the filter picked exactly one real client - the case
// where the screen can speak about that client (what they owe, whose payment the footer
// registers). "Sem cliente registrado" is not a client.
export function singleClientId(clientIds: string): string {
  const ids = parseClientIds(clientIds)
  return ids.length === 1 && ids[0] !== NO_CLIENT ? ids[0] : ""
}

// GROUP_BY_ADMINISTRATOR has to match groupByAdministrator in
// backend/internal/api/v1/handlers/sales.go.
export const GROUP_BY_ADMINISTRATOR = "administrator"
export const GROUP_BY_ACCOUNT = "account"

// The bucket the backend puts accounts nobody administers in. It is not a uuid on purpose,
// so it can never be mistaken for one - which also means it cannot be passed back as an
// administrator_id filter, and the row's own PDF button is hidden for it. Has to match
// noAdministratorKey in backend/internal/api/v1/handlers/sales.go.
export const NO_ADMINISTRATOR_KEY = "administrator:none"

// parseAgencyIds turns the URL's agency_ids into the list the picker holds. Both "all" and
// an absent value come back empty, because the picker draws them the same way - the
// difference between them only matters on the way out, in buildSalesQuery.
export function parseAgencyIds(value: string): string[] {
  if (!value || value === ALL_AGENCIES) return []
  return value.split(",").filter(Boolean)
}

// serializeAgencyIds is the inverse, and is where "none selected" becomes the explicit
// token instead of an empty param that would be dropped and read back as the main agency.
export function serializeAgencyIds(ids: string[]): string {
  return ids.length === 0 ? ALL_AGENCIES : ids.join(",")
}

// Builds the query string for the backend out of the resolved period plus filters. Empty
// dimensions are left out entirely rather than sent as "", which the Go handlers read as
// "no filter" anyway - this just keeps the request URL readable.
export function buildSalesQuery(
  period: ResolvedPeriod,
  filters: SalesFilterState,
): URLSearchParams {
  // No dates at all is the "every issuance" period: the backend reads absent bounds as
  // unfiltered, and the exports follow the same query.
  const query = new URLSearchParams()
  if (period.date_from) query.set("date_from", period.date_from)
  if (period.date_to) query.set("date_to", period.date_to)
  for (const key of SALES_FILTER_KEYS) {
    if (filters[key]) query.set(key, filters[key])
  }
  return query
}
