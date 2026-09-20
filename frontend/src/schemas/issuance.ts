import { z } from "zod"
import { loyaltyProgramSchema } from "@/schemas/loyalty-program"
import { airlineSchema } from "@/schemas/airline"
import { airportSchema } from "@/schemas/airport"
import { accountSchema, accountNicknameSchema } from "@/schemas/account"
import { userSchema } from "@/schemas/user"
import { agencySchema } from "@/schemas/agency"

export const tripTypeSchema = z.enum(["one_way", "round_trip", "multi_leg"])

export const issuanceStatusSchema = z.enum(["incomplete", "complete", "cancelled"])
export type IssuanceStatus = z.infer<typeof issuanceStatusSchema>

export const issuanceFlightSchema = z.object({
  id: z.string(),
  direction: z.enum(["outbound", "inbound"]),
  leg_order: z.number(),
  flight_number: z.string().nullable(),
  origin_iata_code: z.string().nullable(),
  destination_iata_code: z.string().nullable(),
  departure_at: z.string(),
  arrival_at: z.string().nullable(),
  duration_min: z.number().nullable(),
  cabin: z.string().nullable(),
  created_at: z.string(),
})

export type IssuanceFlight = z.infer<typeof issuanceFlightSchema>

export const issuanceSchema = z.object({
  id: z.string(),
  loyalty_program_id: z.string(),
  loyalty_program: loyaltyProgramSchema.nullish(),
  airline_id: z.string(),
  airline: airlineSchema.nullish(),
  program_locator: z.string(),
  airline_locator: z.string().nullable(),
  latam_purchase_number: z.string().nullable(),
  main_passenger_name: z.string().nullable(),
  trip_type: tripTypeSchema.nullable(),
  origin_iata_code: z.string().nullable(),
  origin_airport: airportSchema.nullish(),
  destination_iata_code: z.string().nullable(),
  destination_airport: airportSchema.nullish(),
  points: z.number(),
  value_of_the_thousand: z.number(),
  taxes: z.number().nullable(),
  // The airline's redemption fee this issuance did not pay, because the account it was made
  // on is exempt. Null on every ordinary account and whenever the rule could not be applied -
  // which is not the same as zero, and zero is a real answer: the rule ran and there was no
  // fee to avoid. It does not take part in what the client owes.
  //
  // Tolerated as absent rather than required: the API and the panel are published by
  // independent labels, so this screen can be live for days against a backend that does not
  // send the field yet.
  redemption_fee_saved_cents: z.number().nullish(),
  // An operator typed the amount by hand, which is what stops the next Traveos sync from
  // recomputing over the correction.
  redemption_fee_manual: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  account_id: z.string(),
  account: accountSchema.nullish(),
  account_nickname_id: z.string().nullable(),
  account_nickname: accountNicknameSchema.nullish(),
  issuer_id: z.string(),
  issuer: userSchema.nullish(),
  agency_id: z.string(),
  agency: agencySchema.nullish(),
  complete: z.boolean(),
  // Set only when the itinerary came from Traveos, which is what separates it from
  // an issuance someone marked complete by hand. Gates the ticket page and the PDF.
  traveos_synced_at: z.string().nullish(),
  // Set while somebody (any operator, or the bot) is pulling this issuance from
  // Traveos. The server already drops a stale marker, so a value here means the
  // pull is genuinely in flight.
  traveos_syncing_at: z.string().nullish(),
  // Set whenever the automatic pull ran, found something or not. traveos_synced_at
  // only records the ones that found something, so this is what tells "already
  // looked for and it is not at the vendor" from "never looked".
  traveos_attempted_at: z.string().nullish(),
  // The issuance came from the pre-panel spreadsheet. Its booking was made outside
  // the system, so Traveos may be searched but must never be asked to issue it.
  imported: z.boolean().default(false),
  status: issuanceStatusSchema,
  flights: z.array(issuanceFlightSchema).nullish(),
  observation: z.string().nullable(),
  active: z.boolean(),
  cancelled_at: z.string().nullable(),
  cancelled_by_id: z.string().nullable(),
  refunded_points: z.number().nullable(),
  refunded_taxes: z.number().nullable(),
  cancellation_fee: z.number().nullable(),
  // The row exists only to carry an estorno: the booking was made outside the panel,
  // so the supplier balance counts its refund and not its points.
  cancellation_only: z.boolean().default(false),
  created_at: z.string(),
  // The day of the issuance, as the operator's calendar day. created_at is the same
  // instant, but resolving it to a day belongs on the server: the panel would otherwise
  // do timezone arithmetic that the closing and the filters already do elsewhere.
  issued_on: z.string(),
  updated_at: z.string(),
})

export type Issuance = z.infer<typeof issuanceSchema>

export const getIssuancesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(issuanceSchema).nullish(),
  }),
}

export const createIssuanceSchema = {
  request: z.object({
    program_locator: z.string().min(1, "Localizador é obrigatório"),
    loyalty_program_id: z.string().uuid("Selecione um programa de fidelidade"),
    account_id: z.string().uuid("Selecione uma conta"),
    // Optional: accounts with no registered alias are still issuable.
    account_nickname_id: z.string().uuid().optional(),
    points: z.number().int().positive("Pontos deve ser maior que zero"),
    value_of_the_thousand: z.number().int().positive("Valor do milheiro deve ser maior que zero"),
    airline_id: z.string().uuid().optional(),
    agency_id: z.string().uuid().optional(),
    issuer_id: z.string().uuid().optional(),
    airline_locator: z.string().optional(),
    latam_purchase_number: z.string().optional(),
    main_passenger_name: z.string().optional(),
    trip_type: tripTypeSchema.optional(),
    origin_iata_code: z.string().optional(),
    destination_iata_code: z.string().optional(),
    taxes: z.number().int().optional(),
    // Nullable, not just optional: an explicit null is what releases the issuance back to
    // the automatic calculation, and the backend tells the two apart by key presence.
    redemption_fee_saved_cents: z.number().int().nullable().optional(),
    observation: z.string().optional(),
    // ISO YYYY-MM-DD - converted from the grid's DD/MM/AAAA mask before sending.
    flight_date: z.string().optional(),
    // The day the issuance happened, when it is not today. Omitted means now.
    issued_on: z.string().optional(),
    // A Traveos lookup made before the row existed: the backend writes its itinerary onto
    // the new issuance without asking the vendor again. The ticket code is the fallback
    // for when the server no longer holds the lookup.
    traveos_lookup_id: z.string().optional(),
    traveos_ticket_code: z.string().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: issuanceSchema.nullish(),
  }),
}

// Partial update payload for the grid's autosave - every field is optional, only
// the ones actually edited on a given cell save are sent.
export const updateIssuanceSchema = {
  request: z.object({
    loyalty_program_id: z.string().uuid().optional(),
    airline_id: z.string().uuid().optional(),
    account_id: z.string().uuid().nullable().optional(),
    // Sent alongside account_id, never on its own - the backend updates the pair as a
    // unit, so an omitted alias here means "this account has no alias".
    account_nickname_id: z.string().uuid().nullable().optional(),
    agency_id: z.string().uuid().optional(),
    issuer_id: z.string().uuid().optional(),
    program_locator: z.string().optional(),
    airline_locator: z.string().optional(),
    latam_purchase_number: z.string().optional(),
    main_passenger_name: z.string().optional(),
    trip_type: tripTypeSchema.optional(),
    origin_iata_code: z.string().optional(),
    destination_iata_code: z.string().optional(),
    points: z.number().int().optional(),
    value_of_the_thousand: z.number().int().optional(),
    taxes: z.number().int().optional(),
    // Nullable, not just optional: an explicit null is what releases the issuance back to
    // the automatic calculation, and the backend tells the two apart by key presence.
    redemption_fee_saved_cents: z.number().int().nullable().optional(),
    observation: z.string().optional(),
    status: issuanceStatusSchema.optional(),
    // ISO YYYY-MM-DD - converted from the grid's DD/MM/AAAA mask before sending.
    flight_date: z.string().optional(),
    // Moves the issuance to another day, keeping its time. Refused with 422 when the
    // account's inventory period covering that day is closed.
    issued_on: z.string().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: issuanceSchema.nullish(),
  }),
}

export const deleteIssuanceSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

// Cancel payload - all fields optional (a bare cancel with no refund is valid).
// refunded_taxes/cancellation_fee are in centavos, matching the backend (in cents).
export const cancelIssuanceSchema = {
  request: z.object({
    refunded_points: z.number().int().nonnegative().optional(),
    refunded_taxes: z.number().int().nonnegative().optional(),
    cancellation_fee: z.number().int().nonnegative().optional(),
    // True when the booking was never in the panel and the issuance was registered
    // just to hang this estorno off it. Omitted (false) on an ordinary cancellation.
    cancellation_only: z.boolean().optional(),
    // The day the airline gave the points back, when the cancellation is being recorded
    // after the fact. Omitted means today.
    cancelled_on: z.string().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: issuanceSchema.nullish(),
  }),
}

export type CancelIssuanceInput = z.infer<typeof cancelIssuanceSchema.request>

// Revert a cancellation - no request body; the backend restores active=true, clears the
// cancellation metadata and puts the linked sale back to its pre-cancel state.
export const uncancelIssuanceSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: issuanceSchema.nullish(),
  }),
}

// POST /issuances/{id}/traveos-sync takes no body: the route always forces the
// refetch, because the only thing that reaches it is an operator pressing the
// button, and a deliberate press on an already-synced row means "the flight
// changed", not "retry".
export const lookupTraveosSchema = {
  request: z.object({
    airline_id: z.string().uuid(),
    program_locator: z.string().min(1),
    airline_locator: z.string().optional(),
    origin_iata_code: z.string().optional(),
    main_passenger_name: z.string().optional(),
    latam_purchase_number: z.string().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        lookup_id: z.string(),
        ticket_code: z.string(),
        flight_date: z.string().nullish(),
        origin_iata_code: z.string().nullish(),
        destination_iata_code: z.string().nullish(),
        main_passenger_name: z.string().nullish(),
        latam_purchase_number: z.string().nullish(),
      })
      .nullish(),
  }),
}

export const syncIssuanceWithTraveosSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: issuanceSchema.nullish(),
  }),
}
