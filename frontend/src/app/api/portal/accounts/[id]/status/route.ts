import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// Polled by the portal's connect block while a login runs, to learn when it finishes. The
// browser cannot call the Go API directly, so this attaches the Bearer token and relays the
// sync-state payload unwrapped, like the staff azul-status/smiles-status routes.
export async function GET(
  _request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params
  const token = (await cookies()).get("auth_token")?.value

  const res = await fetch(
    `${env.BACKEND_URL}/api/v1/portal/accounts/${encodeURIComponent(id)}/status`,
    { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
  )

  const body = await res.json().catch(() => null)
  if (!res.ok || !body) {
    return NextResponse.json(
      { error: "Não foi possível ler o estado da conexão." },
      { status: res.ok ? 502 : res.status },
    )
  }
  return NextResponse.json(body.data ?? null, { status: 200 })
}
