import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The issuances page fetches its list server-side, but a live update triggered by an SSE
// event has to refetch from the client, which cannot reach the Go backend on its own.
// This handler forwards the current filter query verbatim so the refreshed list matches
// exactly what the page is showing - and, crucially, so the filter rules stay implemented
// only once, on the server.
export const dynamic = "force-dynamic"

export async function GET(request: Request) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const search = new URL(request.url).search
  const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances${search}`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  if (!res.ok) {
    return NextResponse.json({ error: "Não foi possível carregar as emissões." }, { status: res.status })
  }

  return NextResponse.json(await res.json())
}
