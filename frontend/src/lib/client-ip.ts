import { headers } from "next/headers"

/**
 * The address of the browser that reached the panel, in the shape the Go API
 * expects to receive it.
 *
 * The API has no public address any more: every call it serves arrives from this
 * container over the Compose network, so `r.RemoteAddr` is this container on
 * every request. That is fine for the rate limit that buckets by authenticated
 * user, but login has no user yet - it is throttled per IP, and per IP had
 * quietly become "the whole company shares ten attempts a minute", because one
 * wrong-password loop spends everyone's budget and nothing tells them why.
 *
 * Caddy appends the peer it received the request from to X-Forwarded-For, so the
 * rightmost entry is the only one the client cannot author. That is the entry the
 * API's `middleware.RealIP` reads, and the only one worth forwarding: passing the
 * header through verbatim would hand the API a list whose left-hand side is
 * attacker input.
 *
 * Returns null when there is no header to trust - locally there is no proxy in
 * front, and the API ignores the header entirely unless TRUSTED_PROXY is set.
 */
export async function clientIpHeader(): Promise<Record<string, string>> {
  const forwardedFor = (await headers()).get("x-forwarded-for")
  if (!forwardedFor) return {}

  const rightmost = forwardedFor.split(",").pop()?.trim()
  if (!rightmost) return {}

  return { "X-Forwarded-For": rightmost }
}
