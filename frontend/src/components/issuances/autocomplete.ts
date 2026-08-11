import type { Airline } from "@/schemas/airline"
import type { IssuanceRow } from "@/components/issuances/types"
import {
  ACCOUNT_COLUMN_INDEX,
  LOCATOR_COLUMN_INDEX,
  ORIGIN_COLUMN_INDEX,
  PASSENGER_COLUMN_INDEX,
  POINTS_COLUMN_INDEX,
  VALUE_OF_THE_THOUSAND_COLUMN_INDEX,
} from "@/lib/grid-navigation"

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

/** A column header to light up, and the tooltip explaining why. */
export interface MissingRequirement {
  column: number
  reason: string
}

export type AutocompleteHint =
  // Nothing to say: the toggle is off, or this row is not a candidate for reasons the
  // operator cannot fix by typing (already synced, cancelled, no program picked yet).
  | { kind: "idle" }
  // The row will never autocomplete, however complete it gets.
  | { kind: "unsupported"; airlineName: string }
  | { kind: "missing"; requirements: MissingRequirement[] }

const blank = (value: string | null | undefined) => !(value ?? "").trim()

/**
 * What the selected row is still waiting on before the Traveos autocomplete will fire -
 * isAutocompleteCandidate asked the other way round, so the grid can point at the columns
 * instead of leaving the operator to guess why nothing happened.
 *
 * The two must be read together: any condition added there needs its explanation here, or
 * the panel goes back to silently not firing.
 */
export function autocompleteHintFor(
  row: IssuanceRow | undefined,
  airlines: Airline[],
  enabled: boolean,
): AutocompleteHint {
  if (!enabled || !row) return { kind: "idle" }
  // Painting a checklist over a row that is already synced, cancelled, or being pulled
  // right now would be pointing at work that does not exist.
  if (row.status === "cancelled" || row.traveos_synced_at || row.traveos_syncing_at) {
    return { kind: "idle" }
  }

  // CiaAereaCell always writes an airline alongside the program (the program's default
  // when no "/cia" was typed), so an unresolved one means no program has been picked yet -
  // and until then there is no airline whose rules could be shown.
  const airline = airlines.find((candidate) => candidate.id === row.airline_id)
  if (!airline) return { kind: "idle" }

  if (!airline.traveos_supported || airline.traveos_airline_id == null) {
    return { kind: "unsupported", airlineName: airline.name }
  }

  const requirements: MissingRequirement[] = []

  // The purchase number has no column of its own - it is typed as a further "/" segment
  // inside the Localizador cell - so it shares that header and says so in the tooltip.
  const needsLocator = blank(row.airline_locator) && blank(row.program_locator)
  const needsPurchaseNumber =
    airline.requires_purchase_number && blank(row.latam_purchase_number)
  if (needsLocator || needsPurchaseNumber) {
    requirements.push({
      column: LOCATOR_COLUMN_INDEX,
      reason: needsPurchaseNumber
        ? `A Traveos consulta o bilhete pelo localizador, e a ${airline.name} também exige o número da compra — digitado nesta mesma célula, depois de "/".`
        : "A Traveos consulta o bilhete pelo localizador.",
    })
  }

  if (airline.requires_last_name && blank(row.main_passenger_name)) {
    requirements.push({
      column: PASSENGER_COLUMN_INDEX,
      reason: `A ${airline.name} exige o sobrenome do passageiro para consultar o bilhete.`,
    })
  }

  // Frontend-only: the backend never validates the origin and would send "" to the vendor.
  // So without it the autocomplete simply never fires, with no error to explain it - which
  // is exactly the confusion this hint exists to remove.
  if (blank(row.origin_iata_code)) {
    requirements.push({
      column: ORIGIN_COLUMN_INDEX,
      reason: "A Traveos precisa do aeroporto de origem para localizar o bilhete.",
    })
  }

  // These three are not Traveos requirements at all: they are what draftIsReadyToCreate
  // (types.ts) demands before the row is saved, and the autocomplete only ever looks at
  // saved rows. Change the list there and this has to follow.
  const notSavedYet = "Sem ela a emissão não é salva, e a Traveos só roda em emissão já salva."
  if (row.points === null || row.points <= 0) {
    requirements.push({ column: POINTS_COLUMN_INDEX, reason: notSavedYet })
  }
  if (row.value_of_the_thousand === null || row.value_of_the_thousand <= 0) {
    requirements.push({ column: VALUE_OF_THE_THOUSAND_COLUMN_INDEX, reason: notSavedYet })
  }
  if (row.account_id === null) {
    requirements.push({ column: ACCOUNT_COLUMN_INDEX, reason: notSavedYet })
  }

  return { kind: "missing", requirements }
}
