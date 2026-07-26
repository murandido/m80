import { z } from "zod"

// amount_cents is absent - not null - for anyone who is not an admin: the Go handler drops
// the field entirely rather than sending a zero, so the operator never receives the number
// at all. `.optional()` is what models that; treating a missing amount as 0 here would put
// a convincing "R$ 0,00" on screen instead of hiding the metric.
export const dashboardTotalsSchema = z.object({
  issuance_count: z.number(),
  points: z.number(),
  amount_cents: z.number().optional(),
  cancelled_count: z.number(),
  refunded_points: z.number(),
})

export type DashboardTotals = z.infer<typeof dashboardTotalsSchema>

export const dashboardBucketSchema = z.object({
  // A pre-formatted local key whose shape follows the granularity: "2026-07-01T14:00",
  // "2026-07-01", or "2026-07". Formatted by the server so the axis never re-derives the
  // São Paulo calendar day in the browser and disagrees with the numbers.
  bucket: z.string(),
  issuance_count: z.number(),
  points: z.number(),
  amount_cents: z.number().optional(),
})

export type DashboardBucket = z.infer<typeof dashboardBucketSchema>

export const dashboardBreakdownSchema = z.object({
  id: z.string(),
  name: z.string(),
  // The loyalty program's own hex, for the chip beside the name. Empty for issuers.
  color: z.string().optional(),
  issuance_count: z.number(),
  points: z.number(),
  amount_cents: z.number().optional(),
})

export type DashboardBreakdown = z.infer<typeof dashboardBreakdownSchema>

export const dashboardSchema = z.object({
  range: z.object({
    from: z.string(),
    to: z.string(),
    granularity: z.string(),
  }),
  totals: dashboardTotalsSchema,
  // Absent when the request was open-ended, which is when a "vs. previous period" delta
  // has no meaning.
  previous_totals: dashboardTotalsSchema.optional(),
  series: z.array(dashboardBucketSchema),
  by_program: z.array(dashboardBreakdownSchema),
  by_issuer: z.array(dashboardBreakdownSchema),
})

export type Dashboard = z.infer<typeof dashboardSchema>

export const getDashboardSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: dashboardSchema.nullish(),
  }),
}
