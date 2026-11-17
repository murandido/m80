import { z } from "zod"

export const envSchema = z.object({
  NODE_ENV: z.string(),
  BACKEND_URL: z.string(),
  // The Chrome Web Store link of the M80 Extrato extension, shown on the portal's install guide.
  // Optional: unlisted, so it lives in the server's env rather than in this public repo.
  CLIENT_EXTENSION_STORE_URL: z.string().optional(),
})

export const env = envSchema.parse(process.env)

export type Env = z.infer<typeof env>
