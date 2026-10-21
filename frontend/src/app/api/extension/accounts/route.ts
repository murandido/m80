import { answerPreflight, relayToApi } from "@/lib/extension-relay"

// The vault list for the browser extension: every account with its holder, program and login,
// never a password (see GET /accounts/vault).
export async function GET(request: Request) {
  return relayToApi(request, "/accounts/vault")
}

export function OPTIONS(request: Request) {
  return answerPreflight(request)
}
