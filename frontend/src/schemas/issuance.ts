import { z } from "zod"
import { loyaltyProgramSchema } from "@/schemas/loyalty-program"
import { airlineSchema } from "@/schemas/airline"
import { airportSchema } from "@/schemas/airport"
import { accountSchema } from "@/schemas/account"
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
  account_id: z.string(),
  account: accountSchema.nullish(),
  issuer_id: z.string(),
  issuer: userSchema.nullish(),
  agency_id: z.string(),
  agency: agencySchema.nullish(),
  complete: z.boolean(),
  status: issuanceStatusSchema,
  flights: z.array(issuanceFlightSchema).nullish(),
  observation: z.string().nullable(),
  active: z.boolean(),
  cancelled_at: z.string().nullable(),
  created_at: z.string(),
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
    observation: z.string().optional(),
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
    account_id: z.string().uuid().optional(),
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
    observation: z.string().optional(),
    status: issuanceStatusSchema.optional(),
    // ISO YYYY-MM-DD - converted from the grid's DD/MM/AAAA mask before sending.
    flight_date: z.string().optional(),
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
