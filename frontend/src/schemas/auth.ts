import { z } from "zod"

export const loginSchema = {
  request: z.object({
    phone: z
      .string()
      .min(10, "Telefone deve ter no mínimo 10 dígitos")
      .max(11, "Telefone deve ter no máximo 11 dígitos")
      .regex(/^\d+$/, "Apenas números são permitidos"),
    password: z.string().min(6, {
      message: "A senha precisa ser maior que 6 caracteres",
    }),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        token: z.string(),
      })
      .nullish(),
  }),
}

export type LoginInput = z.infer<typeof loginSchema>
