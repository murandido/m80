// Everything the cadastro listings own in the URL. The URL is the source of truth
// rather than component state, because that is the only thing lib/sticky-filters.ts
// can remember - it stores the query string verbatim in a per-route cookie.
//
// Each screen composes these with whatever filter keys it owns (see LIST_URL_KEYS
// in the page), so the sticky cookie covers the search and the filters together.
export const LIST_BASE_KEYS = ["q", "order", "page"] as const

export type ListSearchParams = Partial<Record<string, string>>

export const LIST_ORDER_KEYS = [
  "name_asc",
  "name_desc",
  "created_desc",
  "created_asc",
] as const

export type ListOrder = (typeof LIST_ORDER_KEYS)[number]

// Airports sort by their own vocabulary: the row leads with the IATA code, so a
// plain "A – Z" would be ambiguous between the code and the name. Kept here beside
// the shared keys so the two whitelists stay visible to each other, and mirroring
// the backend's airportOrder map exactly.
const AIRPORT_SORT_OPTIONS = [
  { value: "code_asc", label: "Código (A – Z)" },
  { value: "code_desc", label: "Código (Z – A)" },
  { value: "name_asc", label: "Nome (A – Z)" },
  { value: "name_desc", label: "Nome (Z – A)" },
] as const

// While searching, the backend ranks by how the row matched - code first, then name,
// city, state, country - and that is the better default. It is offered as a real
// option so the control never claims to be sorting by code while showing you
// relevance; picking anything else overrides the ranking.
export const AIRPORT_RELEVANCE = "relevance"

export function airportOrderOptions(
  isSearching: boolean,
): readonly { value: string; label: string }[] {
  return isSearching
    ? [{ value: AIRPORT_RELEVANCE, label: "Relevância" }, ...AIRPORT_SORT_OPTIONS]
    : AIRPORT_SORT_OPTIONS
}

export function resolveAirportOrder(
  value: string | undefined,
  isSearching: boolean,
): string {
  if (AIRPORT_SORT_OPTIONS.some((option) => option.value === value)) return value as string
  return isSearching ? AIRPORT_RELEVANCE : "code_asc"
}

export function isListOrder(value: string | undefined): value is ListOrder {
  return LIST_ORDER_KEYS.includes(value as ListOrder)
}

export interface ResolvedListParams {
  q: string
  order: ListOrder
  page: number
}

// The backend whitelists `order` too and falls back the same way; resolving it here
// as well keeps the select from rendering a value it cannot show.
export function resolveListParams(params: ListSearchParams): ResolvedListParams {
  return {
    q: params.q?.trim() ?? "",
    order: isListOrder(params.order) ? params.order : "name_asc",
    page: Math.max(1, Number(params.page) || 1),
  }
}

/**
 * Builds the query string sent to the backend, dropping empty dimensions.
 * `extras` carries whatever filters the screen owns beyond search and ordering.
 */
export function listQueryString(
  params: ResolvedListParams,
  extras: Record<string, string | undefined> = {},
): string {
  const query = new URLSearchParams()
  if (params.q) query.set("q", params.q)
  query.set("order", params.order)

  for (const [key, value] of Object.entries(extras)) {
    if (value) query.set(key, value)
  }

  return query.toString()
}
