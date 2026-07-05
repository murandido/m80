import { z } from "zod"

export const agencySchema = z.object({
  id: z.string(),
  name: z.string(),
  is_default: z.boolean(),
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

export const deleteAgencySchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}
