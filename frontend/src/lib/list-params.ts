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
