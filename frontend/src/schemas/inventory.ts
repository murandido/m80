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
  // What the costing says the account still has, and what somebody last read off the
  // airline's page - the only place a real balance exists. Real is null until checked.
  expected_balance_points: z.number(),
  real_balance_points: z.number().nullish(),
  real_balance_checked_on: z.string().nullish(),
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

// --- Conciliação de extrato ---

const statementVerdictSchema = z.enum([
  "matched",
  "missing_in_panel",
  "missing_in_statement",
  "amount_differs",
])

export type StatementVerdict = z.infer<typeof statementVerdictSchema>

const statementBookingLineSchema = z.object({
  locator: z.string(),
  verdict: statementVerdictSchema,
  statement_points: z.number(),
  panel_points: z.number(),
  issuance_id: z.string(),
  occurred_on: z.string(),
})

export type StatementBookingLine = z.infer<typeof statementBookingLineSchema>

const statementRefundLineSchema = z.object({
  locator: z.string(),
  verdict: statementVerdictSchema,
  statement_points: z.number(),
  panel_points: z.number(),
  issuance_id: z.string(),
})

export type StatementRefundLine = z.infer<typeof statementRefundLineSchema>

const statementCreditLineSchema = z.object({
  // Position in this reconciliation, and what the commit takes back. The contents never
  // travel back: the file is read and reconciled again from scratch on commit.
  index: z.number(),
  occurred_on: z.string(),
  points: z.number(),
  description: z.string(),
  verdict: statementVerdictSchema,
  batch_id: z.string(),
})

export type StatementCreditLine = z.infer<typeof statementCreditLineSchema>

export const statementReconciliationSchema = z.object({
  account_number: z.string(),
  from: z.string(),
  to: z.string(),
  bookings: z.array(statementBookingLineSchema),
  refunds: z.array(statementRefundLineSchema),
  credits: z.array(statementCreditLineSchema),
  statement_net_points: z.number(),
  panel_net_points: z.number(),
})

export type StatementReconciliation = z.infer<typeof statementReconciliationSchema>

export const statementReconciliationResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: statementReconciliationSchema.nullish(),
})

export const statementCommitResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: z
    .object({
      // created counts batches, not credits: the commit sums credits of the same origin
      // and payer into one batch, so four instalments come back as one.
      created: z.number(),
      credits: z.number(),
      skipped: z.number(),
    })
    .nullish(),
})

export type StatementCommitResult = z.infer<
  typeof statementCommitResponseSchema
>["data"]

// One credit the operator chose to record. Mirrors schemas.StatementBatchSelection on the
// backend: the position in the reconciliation, plus the two things a statement cannot know
// (what it cost and where it came from). The Smiles commit sends these inside a multipart form;
// the Azul commit sends them as a JSON body.
export interface StatementBatchSelection {
  index: number
  origin_id: string
  cost_cents: number
  funding_agency_id: string
  note: string
}

// Which programme's synced-statement routes to talk to. The two share every shape below; only
// the path segment differs.
export type SyncedProgram = "azul" | "smiles"

// --- Sincronização do extrato (Azul e Smiles) ---
//
// Both programmes are synced and reconciled through the same shapes, because to the panel they
// say the same things. Where one has a field the other does not, the field is optional and the
// comment says which programme it belongs to - that is cheaper than two near-identical screens.

// The lifecycle states, mirroring data.AzulSync* and data.SmilesSync* on the backend. The
// Portuguese labels live on the panel; these are the raw values the API sends.
export const statementSyncKindSchema = z.enum([
  "pending",
  "ok",
  "error",
  // Azul only: the statement did not add up to the balance, so the window was not stored.
  // Smiles publishes no end-of-day balance, so nothing there can disagree.
  "inconsistent",
  // Smiles only: the stored session was refused and only a new verification code fixes it.
  "needs_mfa",
  "needs_attention",
])

export type StatementSyncKind = z.infer<typeof statementSyncKindSchema>

export const statementSyncStateSchema = z.object({
  // Whether the account has been connected. False means it never was (or was disconnected), so
  // the panel offers "Conectar" instead of a running state.
  connected: z.boolean(),
  // Azul only: whether a background connect login is in flight. That login drives a browser and
  // takes minutes, so the panel shows "conectando" and polls until this clears. The Smiles
  // connect answers within the request at every step, so it sends neither field.
  connecting: z.boolean().nullish(),
  connecting_since: z.string().nullish(),
  state: statementSyncKindSchema,
  last_synced_at: z.string().nullish(),
  last_balance_points: z.number().nullish(),
  consecutive_failures: z.number(),
  detail: z.string().nullish(),
  updated_at: z.string().nullish(),
})

export type StatementSyncState = z.infer<typeof statementSyncStateSchema>

// Both reconciliation routes bundle the badge (sync_state) with the reconciliation of the
// synced window, since both come from the database in one read.
export const syncedStatementSchema = z.object({
  sync_state: statementSyncStateSchema,
  reconciliation: statementReconciliationSchema,
})

export type SyncedStatement = z.infer<typeof syncedStatementSchema>

export const syncedStatementResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: syncedStatementSchema.nullish(),
})

// The connect is asynchronous: the server acknowledges that the background login started, and the
// panel watches the sync-state badge for the outcome.
export const azulConnectResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: z.object({ connecting: z.boolean() }).nullish(),
})

// --- Statement page: entries with the resolved issuance link ---

// The kinds a statement line can carry, mirroring data.AzulLine* and data.SmilesLine* on the
// backend. The Portuguese labels live on the statement page.
export const statementLineKindSchema = z.enum([
  "redemption",
  "refund",
  "accrual",
  "purchase",
  // Azul only: the Smiles statement carries no expiry movement.
  "expiration",
  "other",
])

export type StatementLineKind = z.infer<typeof statementLineKindSchema>

// The real issuance a statement line resolves to. manual says whether it was pinned by hand
// (true) or matched automatically by locator (false).
export const statementLinkedIssuanceSchema = z.object({
  issuance_id: z.string(),
  passenger_name: z.string(),
  origin: z.string(),
  destination: z.string(),
  issued_on: z.string(),
  points: z.number(),
  manual: z.boolean(),
})

export type StatementLinkedIssuance = z.infer<typeof statementLinkedIssuanceSchema>

// The point batch (lote) a purchase or accrual line resolves to. manual says whether it was
// pinned by hand (true) or matched automatically by the batch's recorded credits (false).
export const statementLinkedBatchSchema = z.object({
  batch_id: z.string(),
  origin: z.string(),
  acquired_on: z.string(),
  points: z.number(),
  cost_cents: z.number(),
  manual: z.boolean(),
})

export type StatementLinkedBatch = z.infer<typeof statementLinkedBatchSchema>

// One synced statement line as the page shows it: the raw movement plus its resolved issuance
// (null when the line has no locator or nothing matched). An unknown kind from a newer API
// degrades to "other" rather than failing the parse.
export const statementEntrySchema = z.object({
  // The key a manual link is stored under: the locator, or a content signature when the line
  // has none. Sent back to link or unlink the line.
  key: z.string(),
  occurred_on: z.string(),
  // Azul only: the movement's instant in São Paulo time (RFC 3339), null on lines synced before
  // the airline's clock was kept.
  occurred_at: z.string().nullish(),
  kind: statementLineKindSchema.catch("other"),
  type_code: z.string().nullish(),
  type_description: z.string().nullish(),
  points: z.number(),
  locator: z.string(),
  origin: z.string(),
  destination: z.string(),
  partner_name: z.string(),
  description: z.string(),
  // Azul only: free-text detail of a non-flight redemption (the Azul Viagens hotel/product).
  note: z.string().nullish(),
  // What the account held at the end of the day. On both programmes it is derived from these
  // very movements (Azul's own per-day figure is only consistent within one query), so it always
  // adds up: shown, never checked.
  day_balance: z.number().nullish(),
  // Smiles only: the airline's own sub-classification ("Cancellation" on a credit is an estorno)
  // and where the movement was made ("Web", "APP", "MSR").
  sub_type: z.string().nullish(),
  channel: z.string().nullish(),
  linked_issuance: statementLinkedIssuanceSchema.nullish(),
  linked_batch: statementLinkedBatchSchema.nullish(),
})

export type StatementEntry = z.infer<typeof statementEntrySchema>

// Azul only: one operation - the movements of one locator and kind within a minute of each other
// (a round trip for four is eight movements), or a single movement without a locator. A
// redemption and its refund are always separate groups.
export const statementGroupSchema = z.object({
  key: z.string(),
  occurred_on: z.string(),
  occurred_at: z.string().nullish(),
  kind: statementLineKindSchema.catch("other"),
  locator: z.string(),
  points: z.number(),
  line_count: z.number(),
  // Legs in the order they were flown ("CNF→SSA").
  routes: z.array(z.string()).nullish(),
  day_balance: z.number().nullish(),
  // The linked issuance does not add up: the locator's redemptions against its points, or its
  // refunds against its refunded points.
  points_mismatch: z.boolean(),
  linked_issuance: statementLinkedIssuanceSchema.nullish(),
  linked_batch: statementLinkedBatchSchema.nullish(),
  lines: z.array(statementEntrySchema),
})

export type StatementGroup = z.infer<typeof statementGroupSchema>

export const statementEntriesSchema = z.object({
  sync_state: statementSyncStateSchema,
  from: z.string(),
  to: z.string(),
  lines: z.array(statementEntrySchema),
  // Azul only; the Smiles statement is not grouped.
  groups: z.array(statementGroupSchema).nullish(),
})

export type StatementEntries = z.infer<typeof statementEntriesSchema>

export const statementEntriesResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: statementEntriesSchema.nullish(),
})

// The link routes answer with how the locator now resolves, so the page can update the line in
// place without a full refetch.
export const bookingLinkResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: z
    .object({
      locator: z.string(),
      linked_issuance: statementLinkedIssuanceSchema.nullish(),
    })
    .nullish(),
})

// The batch-link routes answer with how the line now resolves to a batch.
export const batchLinkResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: z
    .object({
      key: z.string(),
      linked_batch: statementLinkedBatchSchema.nullish(),
    })
    .nullish(),
})

// --- Conexão da conta Smiles ---
//
// Unlike Azul's one-shot background login, connecting Smiles is a conversation: log in, pick a
// channel, type the code. Each step answers within its own request, so there is no "connecting"
// state to poll - what the panel tracks is which step it is on.

export const smilesAuthenticatorSchema = z.object({
  id: z.string(),
  // "sms" or "email".
  channel: z.string(),
  // Already masked by the server ("***77", "f***@o***"). The real number never reaches here.
  target: z.string(),
})

export type SmilesAuthenticator = z.infer<typeof smilesAuthenticatorSchema>

export const smilesConnectResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: z
    .object({
      connected: z.boolean(),
      mfa_required: z.boolean(),
      authenticators: z.array(smilesAuthenticatorSchema).nullish(),
    })
    .nullish(),
})

export const smilesChallengeResponseSchema = z.object({
  success: z.boolean(),
  message: z.string(),
  data: z
    .object({
      channel: z.string(),
      sent_to: z.string(),
    })
    .nullish(),
})
