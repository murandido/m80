import { cookies } from "next/headers"
import { NextResponse } from "next/server"

export async function GET() {
  const cookieStore = await cookies()

  cookieStore.delete("auth_token")

  // A relative Location on purpose: the standalone server builds request.url from
  // its bind address (HOSTNAME=0.0.0.0), not from the Host Caddy forwards, so an
  // absolute redirect sent people to https://0.0.0.0:3000/.
  return new NextResponse(null, { status: 303, headers: { Location: "/" } })
}
