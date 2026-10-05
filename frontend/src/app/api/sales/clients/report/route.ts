import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// Same job as the suppliers report handler next door: the Go backend reads the JWT from
// the Authorization header, which a browser download cannot send. The filter query string
// is forwarded verbatim, so the statement covers exactly the list on screen.
export async function GET(request: Request) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const search = new URL(request.url).search
  const backendUrl = `${env.BACKEND_URL}/api/v1/sales/clients/report${search}`

  const res = await fetch(backendUrl, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  if (!res.ok) {
    // The backend's message says why (no issuance in the filter, PDF service down), which
    // is what the operator needs to read in the toast.
    const message = await res
      .json()
      .then((body) => body?.message)
      .catch(() => null)

    return NextResponse.json(
      { error: message || "Não foi possível gerar o PDF." },
      { status: res.status },
    )
  }

  const headers = new Headers()
  const contentType = res.headers.get("Content-Type")
  const disposition = res.headers.get("Content-Disposition")
  if (contentType) headers.set("Content-Type", contentType)
  if (disposition) headers.set("Content-Disposition", disposition)

  return new NextResponse(res.body, { status: 200, headers })
}
