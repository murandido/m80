// Remembers the filters an operator picked on a screen for as long as the browser
// session lasts, so walking from Emissões to Vendas and back does not reset the
// screen to its defaults.
//
// A session cookie rather than sessionStorage on purpose: the Server Component
// reads it, so the very first paint is already filtered. With sessionStorage the
// page would render the default list and swap it out a moment later.
//
// Nothing secret lives here - these are ids the operator just chose on screen -
// so the client writes it with document.cookie and no HttpOnly.

const COOKIE_PREFIX = "m80_filters_"

// One memory per screen, keyed by its path: /dashboard/sales and
// /dashboard/sales/suppliers share a filter bar but not the same set of keys.
export function stickyFilterCookieName(pathname: string): string {
  return COOKIE_PREFIX + pathname.replace(/[^a-zA-Z0-9]+/g, "_").replace(/^_|_$/g, "")
}

interface CookieReader {
  get(name: string): { value: string } | undefined
}

// Server side: the query string this screen was last left with, or null.
export function readStickyFilters(
  cookieStore: CookieReader,
  pathname: string,
): URLSearchParams | null {
  const raw = cookieStore.get(stickyFilterCookieName(pathname))?.value
  return parseStickyFilters(raw)
}

// Client side: same thing, straight from document.cookie.
export function readStickyFiltersFromDocument(pathname: string): string | null {
  const name = stickyFilterCookieName(pathname)
  const entry = document.cookie
    .split("; ")
    .find((candidate) => candidate.startsWith(`${name}=`))

  return parseStickyFilters(entry?.slice(name.length + 1))?.toString() ?? null
}

function parseStickyFilters(raw: string | undefined): URLSearchParams | null {
  if (!raw) return null
  try {
    const params = new URLSearchParams(decodeURIComponent(raw))
    return params.size > 0 ? params : null
  } catch {
    // A hand-edited cookie is not worth a 500 on the whole page.
    return null
  }
}

// One working day. A session cookie would be the obvious choice and is not good
// enough: closing a tab never ends the session, only quitting the browser does,
// and Chrome hands session cookies back when it restores tabs ("continue where
// you left off", Ctrl+Shift+T). Filters were surviving for days that way. An
// explicit lifetime is the only one we control, and it guarantees the thing that
// actually matters - "Hoje + eu" on a fresh morning, never yesterday's filters.
const MAX_AGE_SECONDS = 8 * 60 * 60

export function writeStickyFilters(pathname: string, query: string): void {
  document.cookie = `${stickyFilterCookieName(pathname)}=${encodeURIComponent(query)}; path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax`
}

export function clearStickyFilters(pathname: string): void {
  document.cookie = `${stickyFilterCookieName(pathname)}=; path=/; Max-Age=0; SameSite=Lax`
}

// Falls back to the remembered filters only when the URL carries none of `keys`.
// Once any key is in the URL it stays authoritative, exactly as before - which is
// what every control assumes when it clears a single dimension.
export function stickySearchParams<K extends string>(
  params: Partial<Record<K, string>>,
  remembered: URLSearchParams | null,
  keys: readonly K[],
): Partial<Record<K, string>> {
  if (!remembered) return params
  if (keys.some((key) => params[key] !== undefined)) return params

  const restored: Partial<Record<K, string>> = {}
  for (const key of keys) {
    const value = remembered.get(key)
    if (value !== null) restored[key] = value
  }
  return Object.keys(restored).length > 0 ? restored : params
}
