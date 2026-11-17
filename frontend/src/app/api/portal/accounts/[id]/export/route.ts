import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// The statement client's CSV download. Same reason as the staff route next door: a plain
// <a download> cannot carry the Bearer token the Go API wants. The API picks Azul or Smiles from
// the account itself and refuses an account that was not granted, so nothing here decides access.
export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params
  const token = (await cookies()).get("auth_token")?.value

  // Only plain YYYY-MM-DD days are forwarded; anything else never reaches the backend URL.
  const searchParams = new URL(request.url).searchParams
  const period = new URLSearchParams()
  for (const name of ["date_from", "date_to"]) {
    const value = searchParams.get(name)
    if (value && /^\d{4}-\d{2}-\d{2}$/.test(value)) period.set(name, value)
  }
  const query = period.size > 0 ? `?${period.toString()}` : ""

  const res = await fetch(
    `${env.BACKEND_URL}/api/v1/portal/accounts/${encodeURIComponent(id)}/export${query}`,
    { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
  )

  if (!res.ok) {
    return NextResponse.json({ error: "Não foi possível gerar o CSV." }, { status: res.status })
  }

  const headers = new Headers()
  const contentType = res.headers.get("Content-Type")
  const disposition = res.headers.get("Content-Disposition")
  if (contentType) headers.set("Content-Type", contentType)
  if (disposition) headers.set("Content-Disposition", disposition)

  return new NextResponse(res.body, { status: 200, headers })
}
