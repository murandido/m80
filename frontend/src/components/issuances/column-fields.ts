import { IssuanceRow } from "@/components/issuances/types"

// Per-column "clear this cell" patch, used by the Delete key. Each editable cell
// already owns its field-shape knowledge, but it lives inside the cell components;
// Delete needs it in one place, addressed by column index. Returns null for
// columns that must not be cleared (the calculated total, and Status - an enum
// with no empty value; blanking it would silently un-cancel a row).
export function clearPatchForColumn(c: number): Partial<IssuanceRow> | null {
  switch (c) {
    case 0:
      // Clearing the loyalty program drags its dependent account along, mirroring
      // the reset CiaAereaCell performs when the program actually changes.
      return {
        loyalty_program_id: null,
        airline_id: null,
        account_id: null,
        account_nickname_id: null,
      }
    case 1:
      // program_locator is non-nullable, so it clears to "" rather than null.
      return { program_locator: "", airline_locator: "" }
    case 2:
      return { flight_date: null }
    case 3:
      return { main_passenger_name: null }
    case 4:
      return { origin_iata_code: null }
    case 5:
      return { destination_iata_code: null }
    case 6:
      return { points: null }
    case 7:
      return { value_of_the_thousand: null }
    case 8:
      return { taxes: null }
    case 10:
      return { account_id: null, account_nickname_id: null }
    case 11:
      return { issuer_id: null }
    case 12:
      return { agency_id: null }
    // 9 (calculated total) and 13 (status) intentionally fall through to null.
    default:
      return null
  }
}
