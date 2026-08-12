import { z } from "zod"

export const saleSchema = z.object({
  id: z.string(),
  issuance_id: z.string(),
  client_id: z.string().nullable(),
  client_name: z.string().nullable(),
  paid: z.boolean(),
  program_locator: z.string(),
  main_passenger_name: z.string().nullable(),
  issued_at: z.string(),
  issuer_name: z.string(),
  points: z.number(),
  value_of_the_thousand: z.number(),
  taxes: z.number().nullable(),
  amount_cents: z.number(),
  account_id: z.string(),
  account_label: z.string(),
  loyalty_program_name: z.string(),
  loyalty_program_color: z.string(),
  // Set means the sale was reopened by the issuance being cancelled. There is no deleted
  // counterpart: deleting an issuance deletes its sale.
  cancelled_at: z.string().nullable(),
})

export type Sale = z.infer<typeof saleSchema>

export const getSalesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(saleSchema).nullish(),
  }),
}

export const updateSaleSchema = {
  // client_id is nullable rather than optional-only: an explicit null is what unlinks the
  // client, and the backend tells the two apart by key presence.
  request: z.object({
    client_id: z.string().nullable().optional(),
    paid: z.boolean().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: saleSchema.nullish(),
  }),
}

// Miles only: the R$ conversion is settled outside the system, per account agreement.
// One line of an account's statement. An issuance shows up as an "issued" line in the
// period it was created and, separately, as a "refund" line in the period it was cancelled:
// miles that were paid for and came back are money owed the other way. Points and taxes
// arrive already signed - negative on a refund - so the table only ever sums.
export const supplierIssuanceSchema = z.object({
  issuance_id: z.string(),
  kind: z.enum(["issued", "refund"]),
  date: z.string(),
  program_locator: z.string(),
  airline_locator: z.string().nullable(),
  origin_iata_code: z.string().nullable(),
  destination_iata_code: z.string().nullable(),
  points: z.number(),
  taxes: z.number().nullable(),
  cost_of_the_thousand: z.number().nullable(),
  taxes_included_in_cost: z.boolean(),
})

export type SupplierIssuance = z.infer<typeof supplierIssuanceSchema>

export const supplierBalanceSchema = z.object({
  account_id: z.string(),
  account_label: z.string(),
  loyalty_program_name: z.string(),
  loyalty_program_color: z.string(),
  administrator_id: z.string().nullable(),
  administrator_name: z.string().nullable(),
  issued_points: z.number(),
  refunded_points: z.number(),
  net_points: z.number(),
  issuances: z.array(supplierIssuanceSchema),
})

export type SupplierBalance = z.infer<typeof supplierBalanceSchema>

export const getSupplierBalancesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(supplierBalanceSchema).nullish(),
  }),
}
