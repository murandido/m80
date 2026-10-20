import { z } from "zod"

export const mcpTokenSchema = z.object({
  id: z.string(),
  name: z.string(),
  last_used_at: z.string().nullable(),
  revoked_at: z.string().nullable(),
  created_at: z.string(),
})

export type McpToken = z.infer<typeof mcpTokenSchema>

export const getMcpTokensSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(mcpTokenSchema).nullish(),
  }),
}

export const createMcpTokenSchema = {
  request: z.object({
    name: z
      .string()
      .trim()
      .min(1, "Nome é obrigatório")
      .max(100, "Nome deve ter no máximo 100 caracteres"),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    // The plaintext token comes back only in this response; the API keeps just its hash.
    data: mcpTokenSchema.extend({ token: z.string() }).nullish(),
  }),
}

export const revokeMcpTokenSchema = {
  response: z.object({
    success: z.boolean().optional(),
    message: z.string(),
    data: mcpTokenSchema.nullish(),
  }),
}
