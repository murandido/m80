import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The Go backend reads the JWT from the Authorization header, not the cookie, so this
// handler exists to attach the Bearer token and stream the generated .xlsx back to the
// browser. The current filter query string is forwarded verbatim so the export matches
// exactly what the sales page is showing.
export async function GET(request: Request) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const search = new URL(request.url).search
  const backendUrl = `${env.BACKEND_URL}/api/v1/sales/export${search}`

  const res = await fetch(backendUrl, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  if (!res.ok) {
    return NextResponse.json({ error: "Não foi possível gerar a planilha." }, { status: res.status })
  }

  // Pass the xlsx body straight through, preserving the content type and the download
  // filename the backend set in Content-Disposition.
  const headers = new Headers()
  const contentType = res.headers.get("Content-Type")
  const disposition = res.headers.get("Content-Disposition")
  if (contentType) headers.set("Content-Type", contentType)
  if (disposition) headers.set("Content-Disposition", disposition)

  return new NextResponse(res.body, { status: 200, headers })
}
