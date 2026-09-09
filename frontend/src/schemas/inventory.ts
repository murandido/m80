import { z } from "zod"

export const pointBatchOriginSchema = z.object({
  id: z.string(),
  name: z.string(),
  active: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type PointBatchOrigin = z.infer<typeof pointBatchOriginSchema>

export const getPointBatchOriginsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(pointBatchOriginSchema).nullish(),
  }),
}

export const pointBatchSchema = z.object({
  id: z.string(),
  account_id: z.string(),
  origin_id: z.string().nullish(),
  origin: pointBatchOriginSchema.nullish(),
  acquired_on: z.string(),
  points: z.number(),
  cost_cents: z.number(),
  funding_agency_id: z.string(),
  funding_agency: z.object({ id: z.string(), name: z.string() }).nullish(),
  profit_share_percent: z.number(),
  opening: z.boolean(),
  settled_at: z.string().nullish(),
  note: z.string().nullish(),
  // Fractional on purpose: 894172 over 632700 points is 1413,2638 centavos per thousand,
  // and rounding it here is the error the batch costing exists to avoid.
  cost_of_the_thousand: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type PointBatch = z.infer<typeof pointBatchSchema>

export const getPointBatchesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(pointBatchSchema).nullish(),
  }),
}

const settlementSchema = z.object({
  agency_id: z.string(),
  agency_name: z.string().nullish(),
  to_agency_id: z.string(),
  to_agency_name: z.string().nullish(),
  amount_cents: z.number(),
})

export type InventorySettlement = z.infer<typeof settlementSchema>

const consumerSchema = z.object({
  agency_id: z.string(),
  agency_name: z.string().nullish(),
  points: z.number(),
  cost_cents: z.number(),
  revenue_cents: z.number(),
  profit_cents: z.number(),
})

export type InventoryConsumer = z.infer<typeof consumerSchema>

const batchResultSchema = z.object({
  batch_id: z.string(),
  origin_name: z.string().nullish(),
  acquired_on: z.string(),
  funding_agency_id: z.string(),
  funding_agency_name: z.string().nullish(),
  profit_share_percent: z.number(),
  settled_at: z.string().nullish(),
  points: z.number(),
  cost_cents: z.number(),
  consumed_points: z.number(),
  charged_cents: z.number(),
  revenue_cents: z.number(),
  profit_cents: z.number(),
  remaining_points: z.number(),
  consumers: z.array(consumerSchema),
  settlements: z.array(settlementSchema),
})

export type InventoryBatchResult = z.infer<typeof batchResultSchema>

const accountClosingSchema = z.object({
  account_id: z.string(),
  account_label: z.string().nullish(),
  starts_on: z.string(),
  ends_on: z.string(),
  closed: z.boolean(),
  closed_period_id: z.string().nullish(),
  // More than two partner agencies: the single frozen percent cannot express how the
  // profit divides, so the settlement figures are not to be acted on. Absent reads as
  // false, the case for every account today.
  unsupported_split: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  batches: z.array(batchResultSchema),
  pending_points: z.number(),
  cost_cents: z.number(),
  revenue_cents: z.number(),
  profit_cents: z.number(),
  settlements: z.array(settlementSchema),
})

export type InventoryAccountClosing = z.infer<typeof accountClosingSchema>

export const inventoryClosingSchema = z.object({
  accounts: z.array(accountClosingSchema),
  totals: z.object({
    cost_cents: z.number(),
    revenue_cents: z.number(),
    profit_cents: z.number(),
    pending_points: z.number(),
    settlements: z.array(settlementSchema),
  }),
})

export type InventoryClosing = z.infer<typeof inventoryClosingSchema>

export const getInventoryClosingSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: inventoryClosingSchema.nullish(),
  }),
}

export const inventoryPeriodSchema = z.object({
  id: z.string(),
  account_id: z.string(),
  starts_on: z.string(),
  ends_on: z.string(),
  closed_at: z.string().nullish(),
  closed_by_id: z.string().nullish(),
  created_at: z.string(),
})

export type InventoryPeriod = z.infer<typeof inventoryPeriodSchema>

export const getInventoryPeriodsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(inventoryPeriodSchema).nullish(),
  }),
}
