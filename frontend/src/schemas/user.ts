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
