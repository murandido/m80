import { z } from "zod"

export const botConnectionStateSchema = z.enum(["disconnected", "pairing", "connected"])
export const botPairMethodSchema = z.enum(["phone", "qr"])

export const botStatusSchema = z.object({
  state: botConnectionStateSchema,
  jid: z.string().optional(),
  pair_method: botPairMethodSchema.optional(),
  pair_code: z.string().optional(),
  updated_at: z.string(),
})

export type BotStatus = z.infer<typeof botStatusSchema>
export type BotPairMethod = z.infer<typeof botPairMethodSchema>

export const getBotStatusSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: botStatusSchema.nullish(),
  }),
}

export const startBotPairingSchema = {
  request: z.object({
    method: botPairMethodSchema.optional(),
    phone: z.string().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: botStatusSchema.nullish(),
  }),
}

export const cancelBotPairingSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: botStatusSchema.nullish(),
  }),
}

export const logoutBotSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: botStatusSchema.nullish(),
  }),
}
