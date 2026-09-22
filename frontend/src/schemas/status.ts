import { z } from "zod"

// A dependency is up, down, deliberately not configured on this server, or not
// probed yet. "not_configured" is not a failure: a deployment without Traveos
// credentials is a supported state, and painting it red teaches everyone to
// ignore the light.
export const dependencyStateSchema = z.enum([
  "up",
  "down",
  "not_configured",
  "unknown",
])

export const dependencyStatusSchema = z.object({
  state: dependencyStateSchema,
  // Only the bot sets this - the WhatsApp session state, which fails
  // independently from the process being alive. Deliberately loose: the
  // backend owns the vocabulary and a value this enum has not heard of should
  // degrade, not blow up the header on every page.
  session: z.string().optional(),
  checked_at: z.string(),
})

export const systemStatusSchema = z.object({
  bot: dependencyStatusSchema,
  traveos: dependencyStatusSchema,
  // One entry per airline the panel reads directly, keyed by the integration's
  // name ("azul", "gol"). Empty until the first carrier is migrated off the
  // vendor, and defaulted rather than required so a panel deployed ahead of the
  // backend that adds it does not fail validation and blank the whole header.
  providers: z.record(z.string(), dependencyStatusSchema).default({}),
})

export type DependencyState = z.infer<typeof dependencyStateSchema>
export type DependencyStatus = z.infer<typeof dependencyStatusSchema>
export type SystemStatus = z.infer<typeof systemStatusSchema>

export const getSystemStatusSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: systemStatusSchema.nullish(),
  }),
}
