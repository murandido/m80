import { z } from "zod"

export const envSchema = z.object({
  NODE_ENV: z.string(),
  BACKEND_URL: z.string(),
})

export const env = envSchema.parse(process.env)

export type Env = z.infer<typeof env>
