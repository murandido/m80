import { z } from "zod"

// Shapes of the temporary spreadsheet importer's two endpoints. They mirror
// backend/internal/legacyimport; delete this file along with the rest of the feature.

// What the preview says about one row: whether it will be written, and if not, why not.
// Only "ok" rows come ticked.
export const legacyImportVerdictSchema = z.enum([
  "ok",
  // An active issuance already carries this locator - nearly always something an
  // operator registered through the panel, which is exactly what should not be
  // imported a second time.
  "already_imported",
  "needs_mapping",
  "duplicate_in_file",
  "invalid",
  // A negative line: an estorno. It will be written like the others, but what it
  // writes is a cancellation - see `cancellation` for which of the two shapes.
  "cancellation",
])

export type LegacyImportVerdict = z.infer<typeof legacyImportVerdictSchema>

// What an estorno line will do. Present only on a "cancellation" row.
export const legacyImportCancellationSchema = z.object({
  // "cancel_existing": an issuance already carries this locator, so it is cancelled the
  // same way the panel's own button cancels one. "create_and_cancel": nothing carries
  // it - the day it was issued was never imported - so it is created and then cancelled.
  action: z.enum(["cancel_existing", "create_and_cancel"]),
  target_issuance_id: z.string(),
  refunded_points: z.number(),
  refunded_taxes: z.number().nullish(),
  // What the issuance being cancelled is worth, when it is already in the database.
  original_points: z.number().nullish(),
  // The estorno gives back less than the issuance is worth - one leg, one passenger.
  // The whole issuance is still cancelled: the panel has no half-cancelled state.
  partial: z.boolean(),
})

export type LegacyImportCancellation = z.infer<typeof legacyImportCancellationSchema>

export const legacyImportRowSchema = z.object({
  block_index: z.number(),
  // The row's line in the sheet, so the operator can find it in the original file.
  number: z.number(),
  verdict: legacyImportVerdictSchema,
  reason: z.string(),
  // Things worth seeing that do not stop the row - today, a dropped airport code.
  notes: z.array(z.string()).nullish(),
  cancellation: legacyImportCancellationSchema.nullish(),

  loyalty_program_id: z.string(),
  loyalty_program_name: z.string(),
  airline_name: z.string(),
  program_locator: z.string(),
  airline_locator: z.string(),
  flight_date: z.string().nullish(),
  main_passenger_name: z.string(),
  origin_iata_code: z.string(),
  destination_iata_code: z.string(),
  points: z.number(),
  value_of_the_thousand: z.number(),
  taxes: z.number().nullish(),
  // The alias as the spreadsheet has it, next to the account it reached - the operator
  // is checking one against the other.
  account_label: z.string(),
  account_id: z.string(),
  account_name: z.string(),
  account_nickname_id: z.string().nullish(),
  issuer_name: z.string(),
  issuer_id: z.string(),
  observation: z.string(),
})

export type LegacyImportRow = z.infer<typeof legacyImportRowSchema>

export const legacyImportBlockSchema = z.object({
  index: z.number(),
  issuer_name: z.string(),
  issuer_id: z.string(),
  rows: z.array(legacyImportRowSchema).nullish(),
})

export type LegacyImportBlock = z.infer<typeof legacyImportBlockSchema>

export const legacyImportPendingAccountSchema = z.object({
  program_code: z.string(),
  program_name: z.string(),
  loyalty_program_id: z.string(),
  account_label: z.string(),
  row_count: z.number(),
})

export type LegacyImportPendingAccount = z.infer<typeof legacyImportPendingAccountSchema>

export const legacyImportPendingIssuerSchema = z.object({
  issuer_name: z.string(),
  // A name match and nothing more. A block can be titled after two people, so the
  // operator confirms it.
  suggested_user_id: z.string(),
  row_count: z.number(),
})

export type LegacyImportPendingIssuer = z.infer<typeof legacyImportPendingIssuerSchema>

export const legacyImportDayPreviewSchema = z.object({
  date: z.string(),
  sheet_name: z.string(),
  agency_id: z.string(),
  agency_name: z.string(),
  // How the sheet wrote its Pontos column, "raw" or "thousands". Shown because reading
  // it wrong is a factor of a thousand and nothing else in the panel would catch it.
  points_unit: z.string().nullish(),
  blocks: z.array(legacyImportBlockSchema).nullish(),
  pending_accounts: z.array(legacyImportPendingAccountSchema).nullish(),
  pending_issuers: z.array(legacyImportPendingIssuerSchema).nullish(),
})

export type LegacyImportDayPreview = z.infer<typeof legacyImportDayPreviewSchema>

export const legacyImportDaySummarySchema = z.object({
  date: z.string(),
  sheet_name: z.string(),
  row_count: z.number(),
  blocks: z.number(),
})

export type LegacyImportDaySummary = z.infer<typeof legacyImportDaySummarySchema>

export const legacyImportPreviewSchema = z.object({
  days: z.array(legacyImportDaySummarySchema),
  // Filled only once a day has been chosen.
  preview: legacyImportDayPreviewSchema.nullish(),
})

export type LegacyImportPreview = z.infer<typeof legacyImportPreviewSchema>

export const legacyImportPreviewResponseSchema = z.object({
  success: z.boolean().nullish(),
  message: z.string().nullish(),
  data: legacyImportPreviewSchema.nullish(),
})

export const legacyImportSkippedRowSchema = z.object({
  block_index: z.number(),
  number: z.number(),
  locator: z.string(),
  reason: z.string(),
})

export const legacyImportCommitResultSchema = z.object({
  imported: z.number(),
  skipped: z.array(legacyImportSkippedRowSchema).nullish(),
  // Issuances an estorno line cancelled, and estornos whose issuance had to be created
  // first because it had never been imported. Counted apart from `imported`, which
  // keeps meaning "written and live".
  cancelled: z.number().nullish(),
  created_and_cancelled: z.number().nullish(),
  // Aliases the operator's mappings taught the system, so the next day of the same
  // spreadsheet resolves on its own.
  registered_aliases: z.number(),
})

export type LegacyImportCommitResult = z.infer<typeof legacyImportCommitResultSchema>

export const legacyImportCommitResponseSchema = z.object({
  success: z.boolean().nullish(),
  message: z.string().nullish(),
  data: legacyImportCommitResultSchema.nullish(),
})

// What the operator decided on screen, sent with both calls so the preview shows the
// same resolution the commit will perform.
export interface LegacyImportMappings {
  accounts: { program_code: string; account_label: string; account_id: string }[]
  issuers: { issuer_name: string; user_id: string }[]
  // The day's agency: where a row goes unless it was pointed somewhere else.
  agency_id: string
  // The program a row falls back to when its sheet has no Companhia column at all -
  // two of the operators' workbooks do not have one.
  default_loyalty_program_id: string
  // Per-row overrides. Not resolved by the server for the preview - the operator picks
  // an id from a list this screen already has, so there is nothing to resolve and no
  // reason to re-upload the workbook on every pick.
  row_agencies: { block_index: number; number: number; agency_id: string }[]
}

export interface LegacyImportRowSelection {
  block_index: number
  number: number
}
