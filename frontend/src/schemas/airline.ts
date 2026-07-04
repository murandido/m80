import { z } from "zod"

export const airlineSchema = z.object({
  id: z.string(),
  name: z.string(),
  code: z.string(),
  traveos_supported: z.boolean(),
  requires_last_name: z.boolean(),
  requires_purchase_number: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type Airline = z.infer<typeof airlineSchema>

export const getAirlinesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(airlineSchema).nullish(),
  }),
}

export const createAirlineSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    code: z.string().min(1, "Código é obrigatório"),
    traveos_supported: z.boolean(),
    requires_last_name: z.boolean(),
    requires_purchase_number: z.boolean(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: airlineSchema.nullish(),
  }),
}

export const updateAirlineSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    code: z.string().min(1, "Código é obrigatório"),
    traveos_supported: z.boolean(),
    requires_last_name: z.boolean(),
    requires_purchase_number: z.boolean(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: airlineSchema.nullish(),
  }),
}

export const deleteAirlineSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}
