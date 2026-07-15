import { z } from "zod"

export const clientSchema = z.object({
  id: z.string(),
  name: z.string(),
  phone: z.string().nullable(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type Client = z.infer<typeof clientSchema>

export const getClientsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(clientSchema).nullish(),
  }),
}

export const createClientSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    phone: z.string().trim().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: clientSchema.nullish(),
  }),
}

export const updateClientSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    phone: z.string().trim().optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: clientSchema.nullish(),
  }),
}

export const deleteClientSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}
