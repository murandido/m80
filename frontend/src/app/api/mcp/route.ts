import { NextResponse } from "next/server"
import { clientIpHeader } from "@/lib/client-ip"
import { env } from "@/lib/env"

// The personal MCP server lives in the Go API, which has no public address, so Claude Code
// reaches it through here. This is a transparent pipe, unlike the other relays: MCP speaks
// JSON-RPC, so the body goes through untouched in both directions (no envelope to unwrap) and
// the API's status is the client's status.
//
// The credential is the MCP token in the Authorization header, passed through as is. The panel
// cookie is never read or forwarded: a page the developer happens to visit must not be able to
// drive these tools with the ambient session, and without CORS headers it cannot read the
// answers either.
export const dynamic = "force-dynamic"

const FORWARDED_REQUEST_HEADERS = [
  "authorization",
  "content-type",
  "accept",
  "mcp-protocol-version",
  "mcp-session-id",
]

const FORWARDED_RESPONSE_HEADERS = ["content-type", "mcp-session-id"]

async function relayToMcp(request: Request): Promise<Response> {
  const headers: Record<string, string> = { ...(await clientIpHeader()) }
  for (const name of FORWARDED_REQUEST_HEADERS) {
    const value = request.headers.get(name)
    if (value) headers[name] = value
  }

  const hasBody = request.method === "POST"

  let upstream: Response
  try {
    upstream = await fetch(`${env.BACKEND_URL}/mcp`, {
      method: request.method,
      headers,
      body: hasBody ? await request.text() : undefined,
      cache: "no-store",
    })
  } catch {
    return NextResponse.json({ error: "O M80 não respondeu. Tente de novo." }, { status: 502 })
  }

  const responseHeaders = new Headers({ "Cache-Control": "no-store" })
  for (const name of FORWARDED_RESPONSE_HEADERS) {
    const value = upstream.headers.get(name)
    if (value) responseHeaders.set(name, value)
  }

  return new Response(upstream.body, { status: upstream.status, headers: responseHeaders })
}

export async function POST(request: Request) {
  return relayToMcp(request)
}

// The server is stateless and answers these with 405; they are relayed so the client gets that
// answer from the API rather than a Next 405 that looks the same but says nothing about why.
export async function GET(request: Request) {
  return relayToMcp(request)
}

export async function DELETE(request: Request) {
  return relayToMcp(request)
}
