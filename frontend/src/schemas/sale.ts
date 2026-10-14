import { z } from "zod"

export const saleSchema = z.object({
  id: z.string(),
  issuance_id: z.string(),
  client_id: z.string().nullable(),
  client_name: z.string().nullable(),
  paid: z.boolean(),
  // Nullish rather than nullable: the screen may briefly be live against an API that does
  // not send it yet, and absent reads as "nothing written", which is what it meant before.
  note: z.string().nullish(),
  program_locator: z.string(),
  main_passenger_name: z.string().nullable(),
  issued_at: z.string(),
  issuer_name: z.string(),
  points: z.number(),
  value_of_the_thousand: z.number(),
  taxes: z.number().nullable(),
  amount_cents: z.number(),
  // What the client has actually handed over: the sum of the sale's payments. Can exceed
  // amount_cents (overpaid) or go negative (more was given back than came in).
  paid_cents: z.number().default(0),
  // What is still missing. Zero or less means settled, which is what `paid` reports.
  balance_cents: z.number().default(0),
  // The part of paid_cents that came from the client's lump-sum payments, and the day of
  // the latest of them. The Pago switch cannot take that part back - it is undone in the
  // client's payment history. Defaulted so an older API reads as "none".
  client_payment_cents: z.number().default(0),
  client_payment_paid_on: z.string().nullish(),
  account_id: z.string(),
  account_label: z.string(),
  loyalty_program_name: z.string(),
  loyalty_program_color: z.string(),
  // The airline's cancellation fee on a cancelled sale, which the client always pays: it is
  // what the balance of a cancelled sale still counts. Zero on a live sale.
  cancellation_fee_cents: z.number().default(0),
  // What the payments control set above (positive) or below (negative) the computed price;
  // amount_cents already includes it. computed_amount_cents is the price without it - absent
  // on an older API, where there was no adjustment to tell apart.
  price_adjustment_cents: z.number().default(0),
  computed_amount_cents: z.number().nullish(),
  // What the ticket's alterations add to the price; zero when it was never altered.
  adjustment_cents: z.number().default(0),
  // Listed because the ticket was altered in the period, not issued there. When the booking
  // was already paid, amount_cents and paid_cents describe the alteration alone.
  alteration_day: z.boolean().default(false),
  // Set means the sale was reopened by the issuance being cancelled. There is no deleted
  // counterpart: deleting an issuance deletes its sale.
  cancelled_at: z.string().nullable(),
})

export type Sale = z.infer<typeof saleSchema>

// A lump sum from a client, spread by the backend over their open sales oldest first. What
// no open sale absorbs is the client's credit, taken by their next sales. The payment
// stays in the client's history and can be undone there.
export const createClientPaymentSchema = {
  request: z.object({
    client_id: z.string(),
    amount_cents: z.number().int().positive(),
    paid_on: z.string(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        client_payment_id: z.string(),
        sale_ids: z.array(z.string()),
        applied_cents: z.number(),
        settled_sales: z.number(),
        credit_cents: z.number(),
      })
      .nullish(),
  }),
}

export const clientPaymentAllocationSchema = z.object({
  sale_id: z.string(),
  program_locator: z.string(),
  amount_cents: z.number(),
})

// One lump sum in the client's history. An undone one keeps its row (deleted_at set) and
// no longer covers any sale.
export const clientPaymentHistoryItemSchema = z.object({
  id: z.string(),
  // Whose payment it is - shown when the history covers several clients. Optional so a
  // response from an API that predates it still parses.
  client_id: z.string().optional(),
  client_name: z.string().optional(),
  amount_cents: z.number(),
  paid_on: z.string(),
  created_by_name: z.string(),
  created_at: z.string(),
  deleted_at: z.string().nullable(),
  deleted_by_name: z.string().nullable(),
  allocations: z.array(clientPaymentAllocationSchema),
})

export type ClientPaymentHistoryItem = z.infer<typeof clientPaymentHistoryItemSchema>

export const getClientPaymentsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        credit_cents: z.number(),
        payments: z.array(clientPaymentHistoryItemSchema),
      })
      .nullish(),
  }),
}

export type ClientPaymentHistory = NonNullable<
  z.infer<typeof getClientPaymentsSchema.response>["data"]
>

export const undoClientPaymentSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        sale_ids: z.array(z.string()),
        credit_cents: z.number(),
      })
      .nullish(),
  }),
}

export const getSalesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(saleSchema).nullish(),
  }),
}

// One window of the clients view (GET /sales/page): the sales at [offset, offset + limit)
// of the filtered list, how long the whole list is, and - with one client filtered - what
// that client owes over all of it.
export const getSalesPageSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.object({
      sales: z.array(saleSchema),
      total: z.number(),
      offset: z.number(),
      limit: z.number(),
      owed_cents: z.number().default(0),
    }),
  }),
}

export const updateSaleSchema = {
  // client_id is nullable rather than optional-only: an explicit null is what unlinks the
  // client, and the backend tells the two apart by key presence.
  request: z.object({
    client_id: z.string().nullable().optional(),
    paid: z.boolean().optional(),
    // Same rule as client_id: null (or "") clears it, absent leaves it alone.
    note: z.string().nullable().optional(),
    // The total the client pays. The server keeps its difference from the computed price,
    // so sending the computed price back clears the adjustment.
    amount_cents: z.number().int().nonnegative().optional(),
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
  account_id: z.string().nullish(),
  account_label: z.string().nullish(),
  kind: z.enum(["issued", "refund"]),
  // Set when the line is an alteration to the ticket rather than the booking or its
  // cancellation. It still arrives as "issued" (miles charged) or "refund" (miles given
  // back), so the sums above need no case of their own.
  adjustment_id: z.string().nullish(),
  adjustment_reason: z.string().nullish(),
  date: z.string(),
  program_locator: z.string(),
  airline_locator: z.string().nullable(),
  origin_iata_code: z.string().nullable(),
  destination_iata_code: z.string().nullable(),
  points: z.number(),
  taxes: z.number().nullable(),
  cost_of_the_thousand: z.number().nullable(),
  // What the points inventory actually charged this line, present only on an account that
  // owns its points. It wins over the rate beside it, which is this figure rounded to a
  // whole centavo per thousand for display - see supplierLineAmount. Nullish because an
  // ordinary account has no batches, and because the backend may not send it yet.
  cost_cents: z.number().nullish(),
  // Tells the screen the rate came from the batches and must not be offered for editing.
  // An inventory booking no batch covers yet is false here and is typed like any other.
  // Absent reads as false, which is what every line meant before the inventory existed.
  cost_from_inventory: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  // The account owns its points. No longer what locks the field - see cost_from_inventory.
  owns_point_inventory: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  taxes_included_in_cost: z.boolean(),
  // Tolerated as absent, not required: the API and the panel are published by independent
  // labels, so this screen can be live for days against a backend that does not send the
  // field yet. Missing reads as "not checked", which is what every row meant before it.
  reviewed: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
})

export type SupplierIssuance = z.infer<typeof supplierIssuanceSchema>

export const supplierBalanceSchema = z.object({
  // Identifies the row itself, whichever grouping was asked for. Tolerated as absent so the
  // panel can be live against an API published before it: the account fields are the
  // fallback, which is what every row meant when account was the only grouping.
  group_key: z.string().nullish(),
  group_label: z.string().nullish(),
  account_count: z.number().nullish(),
  account_id: z.string(),
  account_label: z.string(),
  loyalty_program_name: z.string(),
  loyalty_program_color: z.string(),
  administrator_id: z.string().nullable(),
  administrator_name: z.string().nullable(),
  // The administrator's supplier-PDF export count. Present only on an administrator-grouped
  // row; absent otherwise, and on an API published before this field existed.
  export_counter: z.number().nullish(),
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
