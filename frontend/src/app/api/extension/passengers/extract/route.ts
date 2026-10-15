import { answerPreflight, relayToApi } from "@/lib/extension-relay"

// Passenger extraction for the browser extension: pasted text or document photos in, the
// passengers the model read out (see POST /passengers/extract). The body goes through as sent -
// the API validates it - and nothing is kept on the way.
export async function POST(request: Request) {
  const body = await request.text()
  return relayToApi(request, "/passengers/extract", { method: "POST", body })
}

export function OPTIONS(request: Request) {
  return answerPreflight(request)
}
