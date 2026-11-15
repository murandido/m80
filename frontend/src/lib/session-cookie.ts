import { env } from "@/lib/env"

// The panel session lives in one httpOnly cookie, written in two places: at login
// (src/actions/auth.ts) and when the proxy renews it (src/proxy.ts). Both must write
// it identically - a renewal that changed an attribute would leave a second cookie
// behind or shorten the session - so the name and the attributes live here, in a
// module with no next/headers import, which the proxy can load.
//
// The browser extension reads this same cookie by name to send it as a Bearer header,
// so renaming it, or giving it a Domain, breaks the vault.
export const SESSION_COOKIE = "auth_token"

// Matches the Go token lifetime (auth.TokenLifetime): the cookie must not outlive the
// token it carries, nor drop it while it is still good.
const SESSION_MAX_AGE_SECONDS = 60 * 60 * 24 * 7

export const sessionCookieOptions = {
  httpOnly: true,
  // Off in development so the cookie still works over plain http://localhost.
  secure: env.NODE_ENV === "production",
  // "strict" would drop the cookie when the panel is reached from an
  // external link, and there is no OAuth round trip here that would need it.
  sameSite: "lax",
  maxAge: SESSION_MAX_AGE_SECONDS,
} as const

// A token is renewed once it is more than a day old, i.e. when less than six of its
// seven days are left. A day keeps it to roughly one renewal per person per day - the
// proxy runs on every navigation and server action, and renewing on each would cost a
// round trip to the API every time - while still leaving six days of slack, so anyone
// who opens the panel at least once a week is never logged out.
const RENEW_WHEN_SECONDS_LEFT_BELOW = SESSION_MAX_AGE_SECONDS - 60 * 60 * 24

// tokenNeedsRenewal reads exp from the token without verifying the signature. That is
// safe because the answer is only a hint about when to ask: the API verifies the token
// before it renews anything, and a forged exp gains nothing but a request that fails.
// It keys off exp rather than iat because tokens signed before iat existed have none.
//
// An expired or unreadable token is not renewed: the API would refuse it anyway, and
// the existing path (the layout's /auth/me check) already logs that person out.
export function tokenNeedsRenewal(token: string, nowMs = Date.now()): boolean {
  const payload = token.split(".")[1]
  if (!payload) return false

  let exp: unknown
  try {
    exp = JSON.parse(Buffer.from(payload, "base64url").toString("utf8")).exp
  } catch {
    return false
  }
  if (typeof exp !== "number") return false

  const secondsLeft = exp - nowMs / 1000
  return secondsLeft > 0 && secondsLeft < RENEW_WHEN_SECONDS_LEFT_BELOW
}
