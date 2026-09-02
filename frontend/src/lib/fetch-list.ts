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
