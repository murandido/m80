import { z } from "zod"

const userSchema = z.object({
  id: z.string(),
  name: z.string(),
  phone: z.string(),
  role: z.enum(["user", "admin"]),
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
    role: z.enum(["user", "admin"]),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        id: z.string(),
        name: z.string(),
        phone: z.string(),
        role: z.enum(["user", "admin"]),
        created_at: z.string(),
      })
      .nullish(),
  }),
}
