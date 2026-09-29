import { z } from "zod"

export const clientSchema = z.object({
  id: z.string(),
  name: z.string(),
  phone: z.string().nullable(),
  // Null only for the clients the migration could not place - they bought through more than
  // one agency - until a dev assigns one. Defaulted so an older API response still parses.
  agency_id: z.string().nullable().default(null),
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

// GET /clients/manage. The plain /clients stays an array because the sales screen's
// client picker reads it whole.
export const listClientsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        clients: z.array(clientSchema).nullish(),
        total: z.number(),
        page: z.number(),
        limit: z.number(),
      })
      .nullish(),
  }),
}

export type ClientPage = NonNullable<z.infer<typeof listClientsSchema.response>["data"]>

export const createClientSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    phone: z.string().trim().optional(),
    agency_id: z.string().min(1, "Agência é obrigatória"),
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
    agency_id: z.string().min(1, "Agência é obrigatória"),
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
