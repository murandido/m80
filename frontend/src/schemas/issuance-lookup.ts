import { z } from "zod"

// POST /issuances/lookup - the itinerary preview the board's form asks for before the
// issuance exists. See backend internal/issuance/lookup.go.
export const lookupIssuanceBookingSchema = {
  request: z.object({
    airline_id: z.string(),
    program_locator: z.string(),
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
        flight_date: z.string().nullish(),
        origin_iata_code: z.string().nullish(),
        destination_iata_code: z.string().nullish(),
        main_passenger_name: z.string().nullish(),
        latam_purchase_number: z.string().nullish(),
        source: z.string(),
      })
      .nullish(),
  }),
}

export type IssuanceLookupRequest = z.infer<typeof lookupIssuanceBookingSchema.request>
export type IssuanceLookupPreview = NonNullable<
  z.infer<typeof lookupIssuanceBookingSchema.response>["data"]
>
