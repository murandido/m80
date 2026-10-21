import { answerPreflight, relayToApi } from "@/lib/extension-relay"

// One account's password for the browser extension's vault. The API records every call and
// limits it per user; this handler only adds the token and keeps the body to the one field the
// API accepts.
export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  const body = await request.json().catch(() => null)
  const action = body?.action === "fill" ? "fill" : "copy"

  return relayToApi(request, `/accounts/${encodeURIComponent(id)}/credentials`, {
    method: "POST",
    body: JSON.stringify({ action }),
  })
}

export function OPTIONS(request: Request) {
  return answerPreflight(request)
}
