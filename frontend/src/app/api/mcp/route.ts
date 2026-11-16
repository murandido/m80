import { clientIpHeader } from "@/lib/client-ip"
import { env } from "@/lib/env"

// The public door to the personal MCP server. The Go API has no public address, so an MCP
// client (Claude Code, a scheduled routine) reaches it through here; this route only passes
// the request along and the answer back. Authentication is the MCP token in the Bearer header
// and it is checked by the API - this route never reads the session cookie, so no page a
// logged-in operator visits can use their session against it. No CORS on purpose: MCP clients
// are not browsers.
export const dynamic = "force-dynamic"

// The headers the MCP transport depends on. Everything else (cookies above all) stays here.
const FORWARDED_REQUEST_HEADERS = [
  "authorization",
  "content-type",
  "accept",
  "mcp-protocol-version",
  "mcp-session-id",
  "last-event-id",
]

const FORWARDED_RESPONSE_HEADERS = [
  "content-type",
  "mcp-session-id",
  "www-authenticate",
  "retry-after",
  "x-ratelimit-limit",
  "x-ratelimit-remaining",
  "x-ratelimit-reset",
]

async function relay(request: Request): Promise<Response> {
  const headers: Record<string, string> = { ...(await clientIpHeader()) }
  for (const name of FORWARDED_REQUEST_HEADERS) {
    const value = request.headers.get(name)
    if (value) headers[name] = value
  }

  let upstream: Response
  try {
    upstream = await fetch(`${env.BACKEND_URL}/mcp`, {
      method: request.method,
      headers,
      body: request.method === "POST" ? await request.text() : undefined,
      cache: "no-store",
      signal: request.signal,
    })
  } catch {
    return Response.json({ error: "O M80 não respondeu." }, { status: 502 })
  }

  const responseHeaders = new Headers({ "Cache-Control": "no-store" })
  for (const name of FORWARDED_RESPONSE_HEADERS) {
    const value = upstream.headers.get(name)
    if (value) responseHeaders.set(name, value)
  }
  return new Response(upstream.body, { status: upstream.status, headers: responseHeaders })
}

export const POST = relay
export const GET = relay
export const DELETE = relay
