import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The browser cannot open the SSE stream against the Go backend directly: BACKEND_URL is
// server-only, and EventSource has no way to send an Authorization header (the backend
// reads the JWT from that header, never from a cookie). So this handler reads the
// httpOnly auth cookie, attaches the Bearer token and pipes the upstream stream through,
// mirroring what the .xlsx export route already does for a one-shot download.
export const dynamic = "force-dynamic"

// Serverless functions are capped, so the stream is cut at the limit rather than living
// forever. That is fine: EventSource reconnects on its own, and the cut cancels the
// upstream request, which is what unsubscribes the client from the backend hub.
export const maxDuration = 300

export async function GET(request: Request) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const upstream = await fetch(`${env.BACKEND_URL}/api/v1/events`, {
    headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" },
    cache: "no-store",
    // Load-bearing: when the tab closes, this aborts the upstream request, whose
    // cancelled context makes the backend drop the client. Without it the hub would
    // keep a dead connection until the heartbeat write finally fails.
    signal: request.signal,
  })

  if (!upstream.ok || !upstream.body) {
    return NextResponse.json({ error: "Não foi possível abrir o stream de eventos." }, {
      status: upstream.status,
    })
  }

  return new NextResponse(upstream.body, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  })
}
