import { cookies } from "next/headers"
import { NextResponse } from "next/server"
import { env } from "@/lib/env"

// Where Microsoft sends the browser back after the sign-in that lets the API read the Tropas'
// workbooks. This exact path is registered as the redirect URI on the Azure app, so it must
// not move. It only forwards the code and state to the API with the operator's session -
// the API checks the state was issued to that same person - and lands back on the Tropas page.
export const dynamic = "force-dynamic"

// A relative Location on purpose, like /api/logout: the standalone server builds request.url
// from its bind address (HOSTNAME=0.0.0.0), not from the Host Caddy forwards, so an absolute
// redirect sent the operator to http://0.0.0.0:3000 after signing in.
function back(outcome: string): NextResponse {
  return new NextResponse(null, {
    status: 303,
    headers: { Location: `/dashboard/injunction-groups?microsoft=${outcome}` },
  })
}

export async function GET(request: Request) {
  const params = new URL(request.url).searchParams
  if (params.get("error")) {
    // The operator cancelled or Microsoft refused (e.g. consent denied). Nothing to forward.
    return back("cancelled")
  }

  const code = params.get("code")
  const state = params.get("state")
  if (!code || !state) return back("error")

  const token = (await cookies()).get("auth_token")?.value
  if (!token) return new NextResponse(null, { status: 303, headers: { Location: "/" } })

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/microsoft-connection/callback`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ code, state }),
      cache: "no-store",
    })
    return back(res.ok ? "ok" : "error")
  } catch {
    return back("error")
  }
}
