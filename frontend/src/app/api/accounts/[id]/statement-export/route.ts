import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The Go backend reads the JWT from the Authorization header, not the cookie, and a plain
// <a download> cannot send one - so this handler attaches the Bearer token and streams the
// generated CSV back to the browser, preserving the download filename the backend set.
export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  // Which programme's export to fetch. Validated against the two that exist rather than
  // interpolated as given: this segment goes straight into a backend URL.
  const searchParams = new URL(request.url).searchParams
  const requested = searchParams.get("program")
  const program = requested === "smiles" ? "smiles" : "azul"

  // The page's date filter, forwarded only when it is a plain YYYY-MM-DD day: anything else is
  // dropped rather than passed through to the backend URL.
  const period = new URLSearchParams()
  for (const name of ["date_from", "date_to"]) {
    const value = searchParams.get(name)
    if (value && /^\d{4}-\d{2}-\d{2}$/.test(value)) period.set(name, value)
  }
  const query = period.size > 0 ? `?${period.toString()}` : ""

  const backendUrl = `${env.BACKEND_URL}/api/v1/accounts/${encodeURIComponent(
    id,
  )}/${program}-statement/export${query}`

  const res = await fetch(backendUrl, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  })

  if (!res.ok) {
    return NextResponse.json(
      { error: "Não foi possível gerar o CSV." },
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
