import type { Airline } from "@/schemas/airline"
import type { IssuanceRow } from "@/components/issuances/types"

// Whether a row carries everything Traveos is asked with for this airline.
//
// This is the check that decides when the grid fetches on its own, so it has to be
// the conservative one: `POST /tickets/emission` is not idempotent, and firing it
// with a field still empty issues a real ticket at the vendor that answers nothing.
// Every "no" here just means the operator uses the button instead.
export function isAutocompleteCandidate(
  row: IssuanceRow,
  airlines: Airline[],
  attempted: Set<string>,
): boolean {
  if (row.isDraft || !row.id) return false
  // Nothing to fetch for a cancelled row, and nothing to fix on one that already
  // carries Traveos data - a deliberate refetch is what the button is for.
  if (row.status === "cancelled" || row.traveos_synced_at) return false
  // Somebody else (or the bot) is already pulling this one.
  if (row.traveos_syncing_at) return false
  // One attempt per row per page load: a failure will fail again, and each retry is
  // another chance of issuing a real ticket at the vendor for nothing.
  if (attempted.has(row.key)) return false

  const airline = airlines.find((candidate) => candidate.id === row.airline_id)
  if (!airline || !airline.traveos_supported) return false
  // Without the vendor's own id for the carrier the sync can only answer 422, so
  // firing it would be a toast and nothing else.
  if (airline.traveos_airline_id == null) return false

  return hasAirlineMinimumFields(row, airline)
}

export function hasAirlineMinimumFields(row: IssuanceRow, airline: Airline): boolean {
  // The locator and the origin are what identify the booking at the airline. The
  // locator column is never empty on a saved row, but a row can be saved before the
  // origin is typed.
  const locator = (row.airline_locator || row.program_locator || "").trim()
  if (!locator) return false
  if (!(row.origin_iata_code ?? "").trim()) return false

  if (airline.requires_last_name && !(row.main_passenger_name ?? "").trim()) return false

  // The purchase number reaches a row two ways: the bot demands it on line 2 of an
  // EMISSAO for these airlines, and the Localizador cell takes it as a further "/"
  // segment. Either way it is on the record by the time the row is saved.
  if (airline.requires_purchase_number && !(row.latam_purchase_number ?? "").trim()) return false

  return true
}
