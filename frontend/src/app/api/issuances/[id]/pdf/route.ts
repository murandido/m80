import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// Same reason the export route exists: the Go backend reads the JWT from the
// Authorization header, and `<a download>` cannot send one. So the download goes
// through here, which reads the cookie and attaches the Bearer token.
export async function GET(_request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${id}/pdf`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  if (!res.ok) {
    return NextResponse.json({ error: "Não foi possível gerar o PDF do bilhete." }, { status: res.status })
  }

  // Pass the pdf body straight through, keeping the filename the backend built
  // from the locator - it is what the passenger ends up with on disk.
  const headers = new Headers()
  const contentType = res.headers.get("Content-Type")
  const disposition = res.headers.get("Content-Disposition")
  if (contentType) headers.set("Content-Type", contentType)
  if (disposition) headers.set("Content-Disposition", disposition)

  return new NextResponse(res.body, { status: 200, headers })
}
