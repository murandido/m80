import { z } from "zod"

// The user_role Postgres enum, in one place. Every schema below reuses it: if any single
// one of them did not know a role the backend can return, safeParse fails and the whole
// screen (or the session) breaks, not just that field.
export const userRoleSchema = z.enum(["user", "admin", "dev"])

export const userSchema = z.object({
  id: z.string(),
  name: z.string(),
  phone: z.string(),
  role: userRoleSchema,
  created_at: z.string(),
})

export const getUsersSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(userSchema).nullish(),
  }),
}

export type User = z.infer<typeof userSchema>

export const getMeSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: userSchema.nullish(),
  }),
}

export const updateUserSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    phone: z
      .string()
      .min(10, "Telefone deve ter no mínimo 10 dígitos")
      .max(11, "Telefone deve ter no máximo 11 dígitos")
      .regex(/^\d+$/, "Apenas números são permitidos"),
    password: z.union([
      z.literal(""),
      z
        .string()
        .min(6, "A senha precisa ter no mínimo 6 caracteres")
        .max(20, "A senha pode ter no máximo 20 caracteres"),
    ]),
    role: userRoleSchema,
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        id: z.string(),
        name: z.string(),
        phone: z.string(),
        role: userRoleSchema,
        created_at: z.string(),
      })
      .nullish(),
  }),
}

export const deleteUserSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

export const createUserSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    phone: z
      .string()
      .min(10, "Telefone deve ter no mínimo 10 dígitos")
      .max(11, "Telefone deve ter no máximo 11 dígitos")
      .regex(/^\d+$/, "Apenas números são permitidos"),
    password: z
      .string()
      .min(6, "A senha precisa ter no mínimo 6 caracteres")
      .max(20, "A senha pode ter no máximo 20 caracteres"),
    role: userRoleSchema,
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        id: z.string(),
        name: z.string(),
        phone: z.string(),
        role: userRoleSchema,
        created_at: z.string(),
      })
      .nullish(),
  }),
}
