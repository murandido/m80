import { z } from "zod"

// The free-write tools a token may be allowed to run on its own, with what each does on
// screen. Must match mcp.FreeWriteTools in the backend, which validates the list.
export const mcpFreeWriteTools = [
  { value: "record_statement_batches", label: "Lançar lotes a partir do extrato" },
  { value: "add_point_batch", label: "Lançar lote avulso (só se casar com o extrato)" },
  { value: "update_point_batch", label: "Corrigir lote" },
  { value: "create_issuance", label: "Criar emissão" },
  { value: "update_issuance", label: "Editar emissão" },
  { value: "add_adjustment", label: "Registrar alteração de passagem" },
  { value: "update_adjustment", label: "Corrigir alteração de passagem" },
  { value: "sync_issuance_traveos", label: "Completar emissão pela Traveos" },
  { value: "import_spreadsheet_issuances", label: "Importar emissões da planilha da Tropa (só se o extrato confirmar)" },
] as const

export const mcpTokenLimitsSchema = z.object({
  max_batch_points: z.number().nullish(),
  max_batches_per_day: z.number().nullish(),
})

export const mcpTokenSchema = z.object({
  id: z.string(),
  user_id: z.string(),
  user_name: z.string().nullable(),
  name: z.string(),
  // null: every free write runs directly (a person's own token).
  allowed_write_tools: z.array(z.string()).nullish(),
  limits: mcpTokenLimitsSchema.nullish(),
  last_used_at: z.string().nullable(),
  revoked_at: z.string().nullable(),
  created_at: z.string(),
})

export type MCPToken = z.infer<typeof mcpTokenSchema>

export const mcpProposalStatusSchema = z.enum(["pending", "applied", "rejected", "failed", "expired"])

export type MCPProposalStatus = z.infer<typeof mcpProposalStatusSchema>

export const mcpOperationSchema = z.object({
  method: z.string(),
  path: z.string(),
  body: z.unknown().optional(),
})

export const mcpProposalSchema = z.object({
  id: z.string(),
  user_id: z.string(),
  user_name: z.string().nullish(),
  tool: z.string(),
  summary: z.string(),
  operations: z.array(mcpOperationSchema).nullable(),
  preview: z.unknown().nullable(),
  status: mcpProposalStatusSchema,
  result: z.unknown().nullable(),
  created_at: z.string(),
  expires_at: z.string(),
  decided_at: z.string().nullable(),
  decided_by_id: z.string().nullable(),
  decided_by_name: z.string().nullable(),
})

export type MCPProposal = z.infer<typeof mcpProposalSchema>

export const mcpCallSchema = z.object({
  id: z.string(),
  token_id: z.string(),
  token_name: z.string().nullable(),
  tool: z.string(),
  args: z.unknown().nullable(),
  ok: z.boolean(),
  error: z.string().nullable(),
  duration_ms: z.number(),
  created_at: z.string(),
})

export type MCPCall = z.infer<typeof mcpCallSchema>

function envelope<T extends z.ZodTypeAny>(data: T) {
  return z.object({ success: z.boolean(), message: z.string(), data: data.nullish() })
}

export const listMCPTokensSchema = { response: envelope(z.array(mcpTokenSchema)) }

// Empty number inputs come back as "" or NaN; both mean "no ceiling".
const optionalPositive = z.preprocess(
  (value) => (value === "" || value === null || Number.isNaN(value) ? undefined : Number(value)),
  z.number().int("Use um número inteiro").positive("Use um número maior que zero").optional(),
)

export const createMCPTokenSchema = {
  request: z.object({
    name: z.string().trim().min(1, "Dê um nome ao token").max(80, "Até 80 caracteres"),
    // Empty means the caller.
    user_id: z.string().optional(),
    // restricted=false sends null (everything direct); true sends the chosen list, even empty.
    restricted: z.boolean(),
    allowed_write_tools: z.array(z.string()),
    max_batch_points: optionalPositive,
    max_batches_per_day: optionalPositive,
  }),
  response: envelope(mcpTokenSchema.extend({ token: z.string() })),
}

export const revokeMCPTokenSchema = { response: envelope(mcpTokenSchema) }

export const listMCPProposalsSchema = { response: envelope(z.array(mcpProposalSchema)) }

export const decideMCPProposalSchema = { response: envelope(mcpProposalSchema) }

export const listMCPCallsSchema = { response: envelope(z.array(mcpCallSchema)) }

export const mcpReportFindingSchema = z.object({
  account_id: z.string().nullish(),
  kind: z.string(),
  severity: z.enum(["info", "warning", "critical"]),
  detail: z.string(),
})

export const mcpReportActionSchema = z.object({
  tool: z.string(),
  description: z.string(),
  proposal_id: z.string().nullish(),
})

export const mcpReportSchema = z.object({
  id: z.string(),
  token_id: z.string(),
  token_name: z.string().nullable(),
  report_date: z.string(),
  title: z.string(),
  summary: z.string(),
  findings: z.array(mcpReportFindingSchema).nullish().transform((value) => value ?? []),
  actions: z.array(mcpReportActionSchema).nullish().transform((value) => value ?? []),
  created_at: z.string(),
  updated_at: z.string(),
})

export type MCPReport = z.infer<typeof mcpReportSchema>

export const listMCPReportsSchema = { response: envelope(z.array(mcpReportSchema)) }
