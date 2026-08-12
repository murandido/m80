import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The Go backend reads the JWT from the Authorization header, not the cookie, and an
// <a download> cannot send one - so this handler exists purely to attach the Bearer token
// and stream the PDF back. The filter query string is forwarded verbatim so the document
// covers exactly what the suppliers page is showing.
export async function GET(request: Request) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const search = new URL(request.url).search
  const backendUrl = `${env.BACKEND_URL}/api/v1/sales/suppliers/report${search}`

  const res = await fetch(backendUrl, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  if (!res.ok) {
    // The backend's own message is worth surfacing: a 422 says how many issuances are
    // still missing a cost of the thousand, which is the whole point of it refusing.
    const message = await res
      .json()
      .then((body) => body?.message)
      .catch(() => null)

    return NextResponse.json(
      { error: message || "Não foi possível gerar o resumo." },
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
