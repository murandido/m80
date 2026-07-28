import { cache } from "react"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"
import { env } from "@/lib/env"
import { getMeSchema, User } from "@/schemas/user"

// Server-only by construction: next/headers throws if this module is ever pulled
// into a client component. It lives outside src/actions/auth.ts because that file
// is "use server" and every export there must be a plain async function - a
// cache()-wrapped one is not, and the memoization below is the whole point.

// SessionResult keeps apart the two failures that were previously collapsed into
// one boolean, which is what made the panel log people out for no reason:
//
//   unauthenticated - the backend answered, and the answer is no. Log out.
//   unavailable     - the backend did not answer (429, 5xx, network). The session
//                     is untouched and the cookie must survive.
//
// A rate-limited /auth/me used to read as "your session is invalid" and destroy a
// seven-day cookie over a burst of clicking.
type SessionResult =
  | { status: "ok"; user: User }
  | { status: "unauthenticated" }
  | { status: "unavailable" }

// fetchSession resolves the current session once per render. The memoization
// matters: a dashboard render used to ask /auth/me three separate times (layout
// twice, page once), and LiveRefresh replays the whole render on every SSE event,
// which is how six deletes turned into a hundred-plus requests.
export const fetchSession = cache(async (): Promise<SessionResult> => {
  const token = (await cookies()).get("auth_token")?.value

  if (!token) {
    return { status: "unauthenticated" }
  }

  let response: Response
  try {
    response = await fetch(`${env.BACKEND_URL}/api/v1/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
  } catch {
    // Connection refused, DNS failure, timeout - the backend is down, not the session.
    return { status: "unavailable" }
  }

  if (response.status === 401) {
    return { status: "unauthenticated" }
  }
  if (!response.ok) {
    return { status: "unavailable" }
  }

  const body = await response.json().catch(() => null)
  const parsed = getMeSchema.response.safeParse(body)

  // A 200 the client cannot read is a backend problem, not an expired session.
  if (!parsed.success || !parsed.data.data) {
    return { status: "unavailable" }
  }

  return { status: "ok", user: parsed.data.data }
})

// verifySession guards every page under /dashboard.
//
// It only logs out on a definitive "no". When the backend is merely unreachable it
// lets the render through: the page's own data fetches fail and show their errors,
// which is honest, whereas silently destroying the session is not - and the user
// could not log back in anyway while the backend is down. Nothing is exposed by
// failing open here, because every data route re-checks the token server-side.
export async function verifySession(): Promise<true> {
  const session = await fetchSession()

  if (session.status === "unauthenticated") {
    redirect("/api/logout")
  }

  return true
}

// getSessionUser returns the authenticated user, or null when there is no valid
// session or the backend could not be reached. Used for role-aware UI (e.g. hiding
// admin-only navigation).
export async function getSessionUser(): Promise<User | null> {
  const session = await fetchSession()
  return session.status === "ok" ? session.user : null
}

// requireAdmin guards admin-only pages. Backend routes enforce admin independently
// - this is the UI layer of the same rule, which is why it does not bounce anyone
// when the role simply could not be read: a transient 429 should not throw an admin
// out of their own page. Returns null in that case, and the page renders degraded.
export async function requireAdmin(): Promise<User | null> {
  const session = await fetchSession()

  if (session.status === "unauthenticated") {
    redirect("/api/logout")
  }
  if (session.status === "ok" && session.user.role !== "admin") {
    redirect("/dashboard")
  }

  return session.status === "ok" ? session.user : null
}
