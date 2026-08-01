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

  // The grid has no purchase-number column - it is not one of the fields the sheet
  // shows - so a row here can never satisfy an airline that needs one. Rather than
  // fetch and watch it fail at the vendor, those airlines are simply left to the
  // explicit button, which runs against whatever the record actually holds.
  if (airline.requires_purchase_number) return false

  return true
}
