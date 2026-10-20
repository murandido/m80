import { z } from "zod"

export const agencyNicknameSchema = z.object({
  id: z.string(),
  agency_id: z.string(),
  nickname: z.string(),
  created_at: z.string(),
})

export type AgencyNickname = z.infer<typeof agencyNicknameSchema>

export const agencySchema = z.object({
  id: z.string(),
  name: z.string(),
  is_default: z.boolean(),
  nicknames: z.array(agencyNicknameSchema),
  created_at: z.string(),
  updated_at: z.string(),
})

export type Agency = z.infer<typeof agencySchema>

export const getAgenciesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(agencySchema).nullish(),
  }),
}

export const createAgencySchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    is_default: z.boolean(),
    nicknames: z.array(z.string().min(1)).optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: agencySchema.nullish(),
  }),
}

export const updateAgencySchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    is_default: z.boolean(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: agencySchema.nullish(),
  }),
}

export const createAgencyNicknameSchema = {
  request: z.object({
    nickname: z.string().min(1, "Apelido é obrigatório"),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: agencyNicknameSchema.nullish(),
  }),
}

export const deleteAgencyNicknameSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

export const checkAgencyNicknameAvailabilitySchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.object({ available: z.boolean() }).nullish(),
  }),
}
