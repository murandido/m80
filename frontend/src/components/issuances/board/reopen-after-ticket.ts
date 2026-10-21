// The ticket page is a navigation away from the board, so coming back remounts it with the
// panel closed - and the operator who went to check the ticket has to find the card again.
// The issuance is remembered for that one trip and taken back once by the board. Session
// storage, not the URL: a query param would stop the board's URL from being bare, which is
// what brings back the remembered filters and search.
const STORAGE_KEY = "issuances:reopen-after-ticket"

export function rememberIssuanceForTicketReturn(issuanceId: string) {
  try {
    window.sessionStorage.setItem(STORAGE_KEY, issuanceId)
  } catch {
    // Storage blocked: the panel just comes back closed, as it did before.
  }
}

// Taken once, so going back to the board later on through the sidebar opens nothing.
export function takeIssuanceForTicketReturn(): string | null {
  try {
    const issuanceId = window.sessionStorage.getItem(STORAGE_KEY)
    window.sessionStorage.removeItem(STORAGE_KEY)
    return issuanceId
  } catch {
    return null
  }
}
