import { z } from "zod"

export const listenedChatStatusSchema = z.enum(["pending", "approved", "denied", "deactivated"])

export const listenedChatSchema = z.object({
  id: z.string(),
  chat_jid: z.string(),
  friendly_name: z.string().nullable(),
  status: listenedChatStatusSchema,
  active: z.boolean(),
  requested_by_id: z.string(),
  requested_by_name: z.string(),
  requested_by_phone: z.string(),
  approved_by_id: z.string().nullable(),
  approved_by_name: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type ListenedChat = z.infer<typeof listenedChatSchema>

export const getPendingListenedChatsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(listenedChatSchema).nullish(),
  }),
}

export const decideListenedChatSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: listenedChatSchema.nullish(),
  }),
}

export const getMonitoredListenedChatsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(listenedChatSchema).nullish(),
  }),
}

export const stopListenedChatSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: listenedChatSchema.nullish(),
  }),
}

export const deactivateListenedChatSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: listenedChatSchema.nullish(),
  }),
}

export const reactivateListenedChatSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: listenedChatSchema.nullish(),
  }),
}
