import { z } from "zod"
import { statementSyncKindSchema } from "@/schemas/inventory"

// One account in the statement client's list (GET /portal/accounts). Never carries the
// password, only whether one is saved.
export const portalAccountSchema = z.object({
  id: z.string(),
  program_id: z.string(),
  program_code: z.string(),
  program_name: z.string(),
  program_color: z.string(),
  holder_name: z.string(),
  login: z.string().nullish(),
  has_password: z.boolean(),
  tracks_statement: z.boolean(),
  connected: z.boolean(),
  // An unknown state from a newer API falls back to "pending" instead of failing the list.
  state: statementSyncKindSchema.catch("pending"),
  last_synced_at: z.string().nullish(),
  last_balance_points: z.number().nullish(),
})

export type PortalAccount = z.infer<typeof portalAccountSchema>

export const getPortalAccountsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(portalAccountSchema).nullish(),
  }),
}

// The statement client changing their own M80 password (PUT /auth/me with only the password
// fields). The bounds are the API's; the confirmation never leaves the browser.
export const changeOwnPasswordSchema = z
  .object({
    current_password: z.string().min(1, "Informe a senha atual"),
    new_password: z
      .string()
      .min(6, "A senha precisa ter no mínimo 6 caracteres")
      .max(20, "A senha pode ter no máximo 20 caracteres"),
    confirm_password: z.string(),
  })
  .refine((data) => data.new_password === data.confirm_password, {
    path: ["confirm_password"],
    message: "As senhas não conferem",
  })

export type ChangeOwnPasswordRequest = z.infer<typeof changeOwnPasswordSchema>
