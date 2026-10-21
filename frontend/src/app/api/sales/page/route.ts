import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The payments control loads its list a window at a time from the browser, as the operator
// scrolls, and the browser cannot reach the Go backend on its own. This forwards the filter
// query and the window (offset/limit) verbatim, so the filter rules stay implemented only
// once, on the server.
export const dynamic = "force-dynamic"

export async function GET(request: Request) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const search = new URL(request.url).search
  const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/page${search}`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  if (!res.ok) {
    return NextResponse.json({ error: "Não foi possível carregar as vendas." }, { status: res.status })
  }

  return NextResponse.json(await res.json())
}
