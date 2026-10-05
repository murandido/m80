import { z } from "zod"

export const personNicknameSchema = z.object({
  id: z.string(),
  person_id: z.string(),
  nickname: z.string(),
  created_at: z.string(),
})

export type PersonNickname = z.infer<typeof personNicknameSchema>

export const personSchema = z.object({
  id: z.string(),
  name: z.string(),
  cpf: z.string().nullable(),
  birth_date: z.string().nullable(),
  email: z.string().nullable(),
  phone: z.string().nullable(),
  zip_code: z.string().nullable(),
  state: z.string().nullable(),
  city: z.string().nullable(),
  address: z.string().nullable(),
  address_number: z.string().nullable(),
  complement: z.string().nullable(),
  neighborhood: z.string().nullable(),
  nicknames: z.array(personNicknameSchema),
  complete: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type Person = z.infer<typeof personSchema>

// The person as a combobox needs her, and nothing else. GET /people?view=picker
// returns this shape: three fields instead of fourteen, which is what keeps the
// pickers from putting 1.36 MB of addresses and CPFs into every page that has one.
export const personOptionSchema = z.object({
  id: z.string(),
  name: z.string(),
  nicknames: z.array(personNicknameSchema),
})

export type PersonOption = z.infer<typeof personOptionSchema>

export const getPeopleOptionsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(personOptionSchema).nullish(),
  }),
}

export const getPeopleSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(personSchema).nullish(),
  }),
}

const optionalTextField = z.string().optional().or(z.literal(""))
const optionalEmailField = z.union([z.literal(""), z.string().email("Email inválido")])

export const createPersonSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    cpf: optionalTextField,
    birth_date: optionalTextField,
    email: optionalEmailField,
    phone: optionalTextField,
    zip_code: optionalTextField,
    state: optionalTextField,
    city: optionalTextField,
    address: optionalTextField,
    address_number: optionalTextField,
    complement: optionalTextField,
    neighborhood: optionalTextField,
    nicknames: z.array(z.string().min(1)).optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: personSchema.nullish(),
  }),
}

export const updatePersonSchema = {
  request: z.object({
    name: optionalTextField,
    cpf: optionalTextField,
    birth_date: optionalTextField,
    email: optionalEmailField,
    phone: optionalTextField,
    zip_code: optionalTextField,
    state: optionalTextField,
    city: optionalTextField,
    address: optionalTextField,
    address_number: optionalTextField,
    complement: optionalTextField,
    neighborhood: optionalTextField,
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: personSchema.nullish(),
  }),
}

// What removing a holder did - or would do, for the delete-preview route: accounts nothing
// refers to are deleted for real, the ones with history archived, and the holder deleted when
// no account names them any more (archived otherwise).
export const personRemovalSchema = z.object({
  accounts_deleted: z.number(),
  accounts_archived: z.number(),
  person_deleted: z.boolean(),
})

export type PersonRemoval = z.infer<typeof personRemovalSchema>

export const deletePersonSchema = {
  response: z.object({
    message: z.string(),
    data: personRemovalSchema.nullish(),
  }),
}

export const createPersonNicknameSchema = {
  request: z.object({
    nickname: z.string().min(1, "Apelido é obrigatório"),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: personNicknameSchema.nullish(),
  }),
}

export const deletePersonNicknameSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

export const checkNicknameAvailabilitySchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.object({ available: z.boolean() }).nullish(),
  }),
}


// What merging a whole row of the accounts screen would do: the other holder absorbed, plus
// every loyalty account in a program the surviving holder already has.
export const mergeRowPreviewSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        accounts_merged: z
          .array(
            z.object({
              program_name: z.string(),
              keep_account_id: z.string(),
              drop_account_id: z.string(),
              drop_login: z.string().nullish(),
              issuances: z.number(),
              colliding_nicknames: z.array(z.string()).nullish(),
            }),
          )
          .nullish(),
        accounts_moved: z.number(),
        administered: z.number(),
        nicknames: z.number(),
        fields_to_fill: z.array(z.string()).nullish(),
        nickname_from_name: z.string(),
      })
      .nullish(),
  }),
}

export type MergeRowPreview = NonNullable<z.infer<typeof mergeRowPreviewSchema.response>["data"]>

export const mergeRowSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.object({ warning: z.string() }).nullish(),
  }),
}
