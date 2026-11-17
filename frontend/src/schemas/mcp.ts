import { z } from "zod"

export const mcpTokenSchema = z.object({
  id: z.string(),
  user_id: z.string(),
  user_name: z.string().nullable(),
  name: z.string(),
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

export const createMCPTokenSchema = {
  request: z.object({
    name: z.string().trim().min(1, "Dê um nome ao token").max(80, "Até 80 caracteres"),
  }),
  response: envelope(mcpTokenSchema.extend({ token: z.string() })),
}

export const revokeMCPTokenSchema = { response: envelope(mcpTokenSchema) }

export const listMCPProposalsSchema = { response: envelope(z.array(mcpProposalSchema)) }

export const decideMCPProposalSchema = { response: envelope(mcpProposalSchema) }

export const listMCPCallsSchema = { response: envelope(z.array(mcpCallSchema)) }
