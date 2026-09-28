import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The account card polls this after a connect, to learn when the first sync landed without the
// operator having to reload. The browser cannot call the Go API directly (it needs the
// Authorization header), so this handler attaches the Bearer token and relays the cheap
// sync-state badge. It deliberately hits /status, not /entries, which would load the whole
// statement just to answer "did the first read finish".
export async function GET(
  _request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const backendUrl = `${env.BACKEND_URL}/api/v1/accounts/${encodeURIComponent(
    id,
  )}/smiles-statement/status`

  const res = await fetch(backendUrl, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  const body = await res.json().catch(() => null)
  if (!res.ok || !body) {
    return NextResponse.json(
      { error: "Não foi possível ler o estado da conexão." },
      { status: res.ok ? 502 : res.status },
    )
  }

  // Relay just the sync-state payload the card consumes; the envelope is unwrapped here so the
  // client component reads the badge directly.
  return NextResponse.json(body.data ?? null, { status: 200 })
}
