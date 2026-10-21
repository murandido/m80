import { NextResponse } from "next/server"
import { clientIpHeader } from "@/lib/client-ip"
import { env } from "@/lib/env"

// The routes under /api/extension serve the browser extension. It reads the panel's auth_token
// cookie through the browser's cookies API and sends it as a Bearer header, and these routes
// accept only that header - never the cookie itself.
//
// Riding the cookie was the first design, and it only worked in Chrome: Firefox sends an
// extension page's fetch as a cross-site CORS request, which drops the SameSite=Lax cookie and
// needs a preflight. A Bearer header sidesteps both, and it also closes the door the cookie left
// open: with no ambient credential, a page the employee happens to visit has nothing to ride.
// That is why answering CORS here is safe - and it is answered only for extension origins anyway.
const EXTENSION_ORIGIN = /^(chrome|moz)-extension:\/\/[a-z0-9-]+$/i

function corsHeaders(request: Request): Record<string, string> {
  const origin = request.headers.get("origin")
  if (!origin || !EXTENSION_ORIGIN.test(origin)) return {}
  return {
    "Access-Control-Allow-Origin": origin,
    "Access-Control-Allow-Methods": "GET, POST",
    "Access-Control-Allow-Headers": "Authorization, Content-Type",
    "Access-Control-Max-Age": "600",
    Vary: "Origin",
  }
}

// The preflight Firefox sends before every call. Anything that is not an extension gets no
// CORS headers, so the browser refuses the real request.
export function answerPreflight(request: Request): Response {
  return new Response(null, { status: 204, headers: corsHeaders(request) })
}

function bearerToken(request: Request): string | null {
  const header = request.headers.get("authorization")
  const match = header?.match(/^Bearer (\S+)$/)
  return match?.[1] ?? null
}

function reply(request: Request, body: unknown, status: number, extra: Record<string, string> = {}) {
  return NextResponse.json(body, { status, headers: { ...corsHeaders(request), ...extra } })
}

// relayToApi forwards one call to the Go API with the extension's token, and unwraps the
// {success, message, data} envelope so the extension reads the payload directly. Errors keep
// their status, which is what the extension branches on (401 = log into the panel, 429 = slow
// down), with the API's message for a human.
export async function relayToApi(request: Request, path: string, init: RequestInit = {}): Promise<NextResponse> {
  const token = bearerToken(request)
  if (!token) {
    return reply(request, { error: "Entre no painel M80 para usar o cofre." }, 401)
  }

  let response: Response
  try {
    response = await fetch(`${env.BACKEND_URL}/api/v1${path}`, {
      ...init,
      headers: {
        ...init.headers,
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
        ...(await clientIpHeader()),
      },
      cache: "no-store",
    })
  } catch {
    return reply(request, { error: "O M80 não respondeu. Tente de novo." }, 502)
  }

  // The API's rate limiter answers in plain text, so this one is worded here.
  if (response.status === 429) {
    return reply(request, { error: "Muitas senhas em pouco tempo. Espere um minuto." }, 429)
  }

  const body = await response.json().catch(() => null)
  if (!response.ok || !body) {
    return reply(
      request,
      { error: body?.message ?? "Não foi possível falar com o M80." },
      response.ok ? 502 : response.status,
    )
  }

  // no-store on the way out as well: a password must not land in any cache between here and
  // the extension.
  return reply(request, body.data ?? null, 200, { "Cache-Control": "no-store" })
}
