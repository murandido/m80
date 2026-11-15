import { NextRequest, NextResponse } from "next/server"
import { env } from "@/lib/env"
import { SESSION_COOKIE, sessionCookieOptions, tokenNeedsRenewal } from "@/lib/session-cookie"
import { loginSchema } from "@/schemas/auth"

export async function proxy(request: NextRequest) {
  const token = request.cookies.get(SESSION_COOKIE)?.value

  const { pathname } = request.nextUrl

  const isLoginPage = pathname === "/"
  const isDashboardRoute = pathname.startsWith("/dashboard")

  // logged and is login page
  if (token && isLoginPage) {
    return NextResponse.redirect(new URL("/dashboard", request.url))
  }

  // not logged and is dashboard route
  if (!token && isDashboardRoute) {
    return NextResponse.redirect(new URL("/", request.url))
  }

  const response = NextResponse.next()

  // Sliding session: a token past its first day is traded for a fresh one, so a person
  // who keeps using the panel is never logged out mid-shift. This is the one place the
  // renewal can live - layouts and pages may not set cookies - and it already sees every
  // navigation, refresh and server action (those are POSTs to the page's route).
  if (token && tokenNeedsRenewal(token)) {
    const renewed = await renewToken(token)
    if (renewed) {
      response.cookies.set(SESSION_COOKIE, renewed, sessionCookieOptions)
    }
  }

  return response
}

// renewToken asks the API for a fresh token and returns null on any failure. A failed
// renewal is never a logout: the current token is still valid for days, the request goes
// through untouched, and the next request simply tries again. Deciding that a session is
// over stays with the layout's /auth/me check, which already tells "no" from "could not
// ask". The timeout keeps a slow API from holding up the navigation it rides on.
async function renewToken(token: string): Promise<string | null> {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    })
    if (!res.ok) return null

    const body = loginSchema.response.safeParse(await res.json())
    return body.success ? (body.data.data?.token ?? null) : null
  } catch {
    return null
  }
}

export const config = {
  matcher: [
    /*
     * Execute the middleware on all routes EXCEPT those that begin with:
     * - api (Next's internal API routes, if any.)
     * - _next/static (static files, JS, CSS)
     * - _next/image (optimized image files)
     * - favicon.ico, sitemap.xml, robots.txt (system files)
     */
    "/((?!api|_next/static|_next/image|favicon.ico|sitemap.xml|robots.txt).*)",
  ],
}
