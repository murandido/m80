import { z } from "zod"

function envelope<T extends z.ZodTypeAny>(data: T) {
  return z.object({ success: z.boolean(), message: z.string(), data: data.nullish() })
}

export const microsoftConnectionSchema = z.object({
  configured: z.boolean(),
  connected: z.boolean(),
  state: z.string().nullish(),
  detail: z.string().nullish(),
  account_name: z.string().nullish(),
  account_email: z.string().nullish(),
  last_ok_at: z.string().nullish(),
  connected_at: z.string().nullish(),
})

export type MicrosoftConnection = z.infer<typeof microsoftConnectionSchema>

export const injunctionGroupSchema = z.object({
  id: z.string(),
  name: z.string(),
  whatsapp_chat_name: z.string().nullish(),
  has_spreadsheet: z.boolean(),
  spreadsheet_hint: z.string().nullish(),
  accounts: z.array(
    z.object({
      account_label: z.string(),
      account_id: z.string(),
      program_code: z.string(),
      program_name: z.string(),
      nicknames: z.string(),
    }),
  ),
  // Agency per issuer: one workbook mixes issuers of different agencies. Empty on a mapping
  // saved before the agency moved here; its rows wait as "needs mapping" until one is picked.
  issuers: z.array(
    z.object({
      issuer_name: z.string(),
      user_id: z.string(),
      user_name: z.string(),
      agency_id: z.string().nullish().transform((value) => value ?? ""),
      agency_name: z.string().nullish().transform((value) => value ?? ""),
    }),
  ),
  created_at: z.string(),
  updated_at: z.string(),
})

export type InjunctionGroup = z.infer<typeof injunctionGroupSchema>

// The injunction accounts a group can map. Built server-side from GET /accounts, which carries
// passwords - only these fields ever reach the browser.
export type InjunctionAccountOption = { id: string; label: string }

// A panel user an Emissor name can map to. agency_ids pre-fills the issuer's agency when the
// user belongs to exactly one; with several the operator has to pick.
export type InjunctionUserOption = { id: string; name: string; agency_ids: string[] }

export const injunctionGroupFormSchema = z.object({
  name: z.string().trim().min(1, "Dê um nome ao grupo").max(80),
  whatsapp_chat_name: z.string().trim().max(120),
  // Empty on edit keeps the stored link; the panel never receives it back.
  spreadsheet_url: z.union([z.literal(""), z.string().trim().url("Cole o link completo da planilha")]),
  accounts: z.array(
    z.object({
      account_label: z.string().trim().min(1, "Rótulo como aparece na coluna Conta"),
      account_id: z.string().min(1, "Escolha a conta"),
    }),
  ),
  issuers: z.array(
    z.object({
      issuer_name: z.string().trim().min(1, "Nome como aparece na coluna Emissor"),
      user_id: z.string().min(1, "Escolha o usuário"),
      agency_id: z.string().min(1, "Escolha a agência das emissões"),
    }),
  ),
})

export type InjunctionGroupForm = z.infer<typeof injunctionGroupFormSchema>

export const checkItemSchema = z.object({
  day: z.string().nullish(),
  block_index: z.number(),
  number: z.number(),
  locator: z.string(),
  program: z.string(),
  account_label: z.string().nullish(),
  points: z.number(),
  value_of_the_thousand: z.number(),
  issuer: z.string().nullish(),
  passenger: z.string().nullish(),
  statement_points: z.number().nullish(),
  detail: z.string(),
})

export type CheckItem = z.infer<typeof checkItemSchema>

export const checkResultSchema = z.object({
  from: z.string(),
  to: z.string(),
  sheet_rows: z.number(),
  statements: z
    .array(z.object({ account_id: z.string(), program: z.string(), from: z.string().nullish(), to: z.string().nullish(), error: z.string().nullish() }))
    .nullish()
    .transform((value) => value ?? []),
  missing_in_panel_confirmed: z.array(checkItemSchema),
  missing_in_panel_unconfirmed: z.array(checkItemSchema),
  differs_from_panel: z.array(checkItemSchema),
  missing_in_sheet_and_panel: z.array(checkItemSchema),
  pending_cancellations: z.array(checkItemSchema),
  invalid: z.array(checkItemSchema),
  needs_mapping: z.array(checkItemSchema),
})

export type CheckResult = z.infer<typeof checkResultSchema>

export const importResultSchema = z.object({
  imported: z.number(),
  cancelled: z.number(),
  created_and_cancelled: z.number(),
  skipped: z
    .array(z.object({ number: z.number(), locator: z.string().nullish(), reason: z.string() }))
    .nullish()
    .transform((value) => value ?? []),
})

export const importPeriodResultSchema = z.object({
  total: importResultSchema,
  days: z.array(z.object({ day: z.string(), error: z.string().nullish() }).passthrough()),
})

export const schemas = {
  connection: envelope(microsoftConnectionSchema),
  start: envelope(z.object({ authorize_url: z.string() })),
  groups: envelope(z.array(injunctionGroupSchema)),
  group: envelope(injunctionGroupSchema),
  check: envelope(checkResultSchema),
  importDay: envelope(importResultSchema),
  importPeriod: envelope(importPeriodResultSchema),
  empty: z.object({ success: z.boolean(), message: z.string(), data: z.unknown().nullish() }),
}
