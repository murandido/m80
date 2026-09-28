import { z } from "zod"

// GET /booking-lookups/report. Temporary, like the backend route it reads: it exists
// to decide when each airline can drop Traveos, and goes away with the vendor.

export const lookupFailureSchema = z.object({
  issuance_id: z.string(),
  locator: z.string(),
  outcome: z.string(),
  error: z.string(),
  answered_by: z.string(),
  created_at: z.string(),
})

export const airlineLookupReportSchema = z.object({
  airline_id: z.string(),
  code: z.string(),
  name: z.string(),
  direct_booking_enabled: z.boolean(),
  traveos_supported: z.boolean(),
  direct_provider: z.string(),
  syncs: z.number(),
  answered_by_direct: z.number(),
  answered_by_traveos: z.number(),
  fell_back_to_traveos: z.number(),
  unanswered: z.number(),
  direct_found: z.number(),
  direct_not_found: z.number(),
  direct_unavailable: z.number(),
  direct_refused: z.number(),
  direct_average_ms: z.number(),
  recent_failures: z.array(lookupFailureSchema),
})

export const directLookupReportResponseSchema = z.object({
  data: z
    .object({
      days: z.number(),
      since: z.string(),
      airlines: z.array(airlineLookupReportSchema),
    })
    .nullish(),
})

export type AirlineLookupReport = z.infer<typeof airlineLookupReportSchema>
export type LookupFailure = z.infer<typeof lookupFailureSchema>
