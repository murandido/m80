import { z } from "zod"

/**
 * Reads one of the API's `{success, message, data}` list envelopes.
 *
 * Returns null when the request or the parse failed, which is deliberately
 * distinct from an empty array: an empty list is a legitimate answer and a
 * failure is not, and the pages tell the two apart to decide whether to render
 * "nada aqui" or leave a control alone.
 *
 * Three dashboard pages had grown their own copy of this - their comments
 * pointed at each other - so it lives here now. `cache` defaults to "no-store" because
 * every screen here shows live operation data - which is also what Next 16 does
 * with an unqualified fetch, so this only writes the intent down.
 */
export async function fetchList<T>(
  url: string,
  schema: z.ZodType<{ data?: T[] | null | undefined }>,
  authHeader: Record<string, string>,
  cache: RequestCache = "no-store",
): Promise<T[] | null> {
  try {
    const res = await fetch(url, { headers: authHeader, cache })
    const parsed = schema.safeParse(await res.json())
    if (!res.ok || !parsed.success) return null
    return parsed.data.data ?? []
  } catch {
    return null
  }
}

/**
 * Reads one of the API's paginated envelopes: `{ <noun>: [...], total, page, limit }`.
 *
 * `pick` names the array, which is the only thing that differs between them - the
 * backend spells it `accounts`, `clients`, `documents`, `people`. Four screens read
 * one of these, so the alternative to the callback is four copies of this function.
 *
 * A failed request comes back as an empty page rather than null. The distinction
 * fetchList draws does not pay for itself here: every caller renders "nada aqui" and
 * a pagination control reading zero, which is what an empty page already produces.
 */
export async function fetchPage<T, D extends { total: number }>(
  url: string,
  schema: z.ZodType<{ data?: D | null | undefined }>,
  pick: (data: D) => T[] | null | undefined,
  authHeader: Record<string, string>,
): Promise<{ items: T[]; total: number }> {
  const empty = { items: [] as T[], total: 0 }
  try {
    const res = await fetch(url, { headers: authHeader, cache: "no-store" })
    const parsed = schema.safeParse(await res.json())
    if (!res.ok || !parsed.success || !parsed.data.data) return empty
    return {
      items: pick(parsed.data.data) ?? [],
      total: parsed.data.data.total,
    }
  } catch {
    return empty
  }
}
