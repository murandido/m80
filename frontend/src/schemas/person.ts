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

// The paginated management listing (GET /people/manage). The plain /people route
// stays unpaginated because it is what feeds the holder, administrator and
// passenger pickers on the accounts, sales and issuances screens.
export const listPeopleSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        people: z.array(personSchema).nullish(),
        total: z.number(),
        page: z.number(),
        limit: z.number(),
      })
      .nullish(),
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

export const deletePersonSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
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

// What a merge would move, asked for before the operator confirms: the action deletes a
// record for good, so the panel has to be able to say what is about to happen.
export const mergePersonPreviewSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        accounts: z.number(),
        nicknames: z.number(),
        fields_to_fill: z.array(z.string()).nullish(),
        nickname_from_name: z.string(),
      })
      .nullish(),
  }),
}

export const mergePersonSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        person: personSchema,
        warning: z.string(),
      })
      .nullish(),
  }),
}

export type MergePersonPreview = NonNullable<
  z.infer<typeof mergePersonPreviewSchema.response>["data"]
>
