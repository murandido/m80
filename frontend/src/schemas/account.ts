import { z } from "zod"

export const loyaltyProgramSummarySchema = z.object({
  id: z.string(),
  name: z.string(),
  code: z.string(),
  color: z.string(),
})

export type LoyaltyProgramSummary = z.infer<typeof loyaltyProgramSummarySchema>

export const accountNicknameSchema = z.object({
  id: z.string(),
  account_id: z.string(),
  loyalty_program_id: z.string(),
  nickname: z.string(),
  created_at: z.string(),
})

export type AccountNickname = z.infer<typeof accountNicknameSchema>

export const accountSchema = z.object({
  id: z.string(),
  loyalty_program_id: z.string(),
  loyalty_program: loyaltyProgramSummarySchema,
  login: z.string().nullable(),
  password: z.string().nullable(),
  holder_id: z.string().nullable(),
  administrator_id: z.string().nullable(),
  nicknames: z.array(accountNicknameSchema),
  complete: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
  // Set only on an archived account, which the account list never returns - it only
  // ever arrives embedded in a past issuance, so an old row can still be labelled
  // with it while the pickers leave it out.
  deleted_at: z.string().nullish(),
})

export type Account = z.infer<typeof accountSchema>

export const getAccountsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(accountSchema).nullish(),
  }),
}

const optionalTextField = z.string().optional().or(z.literal(""))
const optionalUuidField = z.union([z.literal(""), z.string().uuid("UUID inválido")])

export const createAccountSchema = {
  request: z.object({
    loyalty_program_id: z.string().uuid("UUID inválido"),
    login: optionalTextField,
    password: optionalTextField,
    holder_id: optionalUuidField,
    administrator_id: optionalUuidField,
    nicknames: z.array(z.string().min(1)).optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountSchema.nullish(),
  }),
}

export const updateAccountSchema = {
  request: z.object({
    loyalty_program_id: optionalUuidField,
    login: optionalTextField,
    password: optionalTextField,
    holder_id: optionalUuidField,
    administrator_id: optionalUuidField,
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountSchema.nullish(),
  }),
}

export const deleteAccountSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

export const createAccountNicknameSchema = {
  request: z.object({
    loyalty_program_id: z.string().uuid("UUID inválido"),
    nickname: z.string().min(1, "Apelido é obrigatório"),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountNicknameSchema.nullish(),
  }),
}

export const deleteAccountNicknameSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

export const checkAccountNicknameAvailabilitySchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.object({ available: z.boolean() }).nullish(),
  }),
}

// Same idea as mergePersonPreviewSchema: the counts the panel shows before an
// irreversible merge. issuances includes the ones in the grid's trash.
export const mergeAccountPreviewSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        issuances: z.number(),
        nicknames: z.number(),
        fields_to_fill: z.array(z.string()).nullish(),
        colliding_nicknames: z.array(z.string()).nullish(),
      })
      .nullish(),
  }),
}

export const mergeAccountSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountSchema.nullish(),
  }),
}

export type MergeAccountPreview = NonNullable<
  z.infer<typeof mergeAccountPreviewSchema.response>["data"]
>
