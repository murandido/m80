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
