"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { searchAirportsSchema } from "@/schemas/airport"
import { Airport } from "@/schemas/airport"

// Explicit return type is required here: without it, Next.js infers a merged
// type for this "use server" export where `airports` ends up possibly
// undefined, breaking `"airports" in result` narrowing at call sites.
export async function searchAirportsAction(
  query: string,
): Promise<{ error: string } | { success: true; airports: Airport[] }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/airports?q=${encodeURIComponent(query)}`,
      { headers: { Authorization: `Bearer ${token}` } },
    )

    const json = await res.json()
    const parsed = searchAirportsSchema.response.safeParse(json)

    if (!res.ok || !parsed.success) {
      return { error: "Erro ao buscar aeroportos." }
    }

    return { success: true as const, airports: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
