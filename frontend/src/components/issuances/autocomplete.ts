import type { Airline } from "@/schemas/airline"
import type { IssuanceRow } from "@/components/issuances/types"

// Whether a row carries everything Traveos is asked with for this airline.
//
// This is the check that decides when the grid fetches on its own, so it has to be
// the conservative one: `POST /tickets/emission` is not idempotent, and firing it
// with a field still empty issues a real ticket at the vendor that answers nothing.
// Every "no" here just means the operator uses the button instead.
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
