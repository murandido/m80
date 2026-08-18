import { z } from "zod"
import { airlineSchema } from "@/schemas/airline"

export const loyaltyProgramSchema = z.object({
  id: z.string(),
  name: z.string(),
  code: z.string(),
  color: z.string(),
  default_airline_id: z.string(),
  default_airline: airlineSchema.nullish(),
  // The beneficiary cap this program enforces. Null means nobody has researched it,
  // which is not a cap of zero: the accounts screen then shows no counter at all.
  cpf_limit: z.number().nullish(),
  cpf_limit_window: z.enum(["calendar_year", "rolling_12_months"]).nullish(),
  cpf_limit_mode: z.enum(["distinct_people", "per_issuance"]).nullish(),
  created_at: z.string(),
  updated_at: z.string(),
  // Same as accountSchema.deleted_at: only ever set on a program reached through a
  // past issuance, so the grid can label the row without offering the program.
  deleted_at: z.string().nullish(),
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
    // "" means the program has no researched cap. The three move as a unit: the API
    // reads an absent one as cleared, so a limit without a window would be meaningless.
    // Kept as text rather than a number: react-hook-form gives an empty input the empty
    // string, and "" is exactly how the form says "no cap". The action converts it.
    cpf_limit: z
      .union([z.literal(""), z.string().regex(/^\d+$/, "Informe um número inteiro")])
      .optional(),
    cpf_limit_window: z
      .union([z.literal(""), z.enum(["calendar_year", "rolling_12_months"])])
      .optional(),
    cpf_limit_mode: z
      .union([z.literal(""), z.enum(["distinct_people", "per_issuance"])])
      .optional(),
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
    // "" means the program has no researched cap. The three move as a unit: the API
    // reads an absent one as cleared, so a limit without a window would be meaningless.
    // Kept as text rather than a number: react-hook-form gives an empty input the empty
    // string, and "" is exactly how the form says "no cap". The action converts it.
    cpf_limit: z
      .union([z.literal(""), z.string().regex(/^\d+$/, "Informe um número inteiro")])
      .optional(),
    cpf_limit_window: z
      .union([z.literal(""), z.enum(["calendar_year", "rolling_12_months"])])
      .optional(),
    cpf_limit_mode: z
      .union([z.literal(""), z.enum(["distinct_people", "per_issuance"])])
      .optional(),
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
