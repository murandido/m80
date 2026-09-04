import { createHash } from "node:crypto"
import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The airport table is 7850 rows and roughly 1.3 MB of JSON. It used to travel inside
// the issuances page: fetched from Go, parsed, and then serialised again into the RSC
// payload of every single page load, for a dropdown that shows at most fifty of them.
// Under a hundred concurrent operators that was the largest single cost in the Next
// process, and it was paid again on every navigation.
//
// Served from here instead, the browser caches it and the grid loads it once per
// session. The list is a reference table - airports are added by hand, a few times a
// year - so it tolerates being minutes stale in a way live operation data does not.
export const dynamic = "force-dynamic"

// How long a copy stays good, in this process and in the browser. Long enough that a
// burst of logins shares one upstream fetch, short enough that an airport added on the
// management screen shows up in the grid without anyone restarting anything.
const TTL_MS = 5 * 60 * 1000

type CachedAirports = { body: string; etag: string; at: number }

let cache: CachedAirports | null = null
// Concurrent misses share one upstream request rather than each fetching 1.3 MB: with a
// hundred operators arriving together, the difference is one request or a hundred.
let inflight: Promise<CachedAirports | null> | null = null

async function fetchAirports(token: string): Promise<CachedAirports | null> {
  const res = await fetch(`${env.BACKEND_URL}/api/v1/airports/all`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })
  if (!res.ok) return null

  const body = await res.text()
  return {
    body,
    etag: `"${createHash("sha1").update(body).digest("base64url")}"`,
    at: Date.now(),
  }
}

async function readAirports(token: string): Promise<CachedAirports | null> {
  if (cache && Date.now() - cache.at < TTL_MS) return cache

  inflight ??= fetchAirports(token)
    .catch(() => null)
    .finally(() => {
      inflight = null
    })

  const fresh = await inflight
  if (fresh) cache = fresh
  // A failed refresh serves the stale copy rather than an empty dropdown: the operator
  // can type the IATA code either way, but with the list they need not remember it.
  return fresh ?? cache
}

export async function GET(request: Request) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  if (!token) {
    return NextResponse.json({ error: "Não autenticado." }, { status: 401 })
  }

  const airports = await readAirports(token)
  if (!airports) {
    return NextResponse.json(
      { error: "Não foi possível carregar os aeroportos." },
      { status: 502 },
    )
  }

  // private, because the route sits behind the session cookie even though the payload
  // itself is the same for every operator.
  const headers = {
    "Cache-Control": "private, max-age=300, stale-while-revalidate=3600",
    ETag: airports.etag,
  }

  if (request.headers.get("if-none-match") === airports.etag) {
    return new NextResponse(null, { status: 304, headers })
  }

  return new NextResponse(airports.body, {
    status: 200,
    headers: { ...headers, "Content-Type": "application/json" },
  })
}
