import { z } from "zod"
import { airlineSchema } from "@/schemas/airline"

export const loyaltyProgramSchema = z.object({
  id: z.string(),
  name: z.string(),
  code: z.string(),
  color: z.string(),
  default_airline_id: z.string(),
  default_airline: airlineSchema.nullish(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type LoyaltyProgram = z.infer<typeof loyaltyProgramSchema>

export const getLoyaltyProgramsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(loyaltyProgramSchema).nullish(),
  }),
}

const hexColorField = z
  .string()
  .regex(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/, "Cor inválida")

export const createLoyaltyProgramSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    code: z.string().min(1, "Código é obrigatório"),
    color: hexColorField,
    default_airline_id: z.string().uuid("Selecione uma companhia aérea"),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: loyaltyProgramSchema.nullish(),
  }),
}

export const updateLoyaltyProgramSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    code: z.string().min(1, "Código é obrigatório"),
    color: hexColorField,
    default_airline_id: z.string().uuid("Selecione uma companhia aérea"),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: loyaltyProgramSchema.nullish(),
  }),
}

export const deleteLoyaltyProgramSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}
