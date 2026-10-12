"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  lookupIssuanceBookingSchema,
  type IssuanceLookupRequest,
} from "@/schemas/issuance-lookup"

// Asks the airline's providers about a booking that has no issuance yet. Nothing is saved
// on our side; the vendor may import the booking, which the sync after the save then reads.
export async function lookupIssuanceBookingAction(data: IssuanceLookupRequest) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/lookup`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
      // Same as the sync: a vendor import takes tens of seconds and must never be
      // answered from a cache.
      cache: "no-store",
    })

    const json = await res.json()
    const parsed = lookupIssuanceBookingSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao buscar a reserva." }
    }
    const preview = parsed.data?.data
    if (!preview) {
      return { error: "Erro ao buscar a reserva." }
    }
    return { success: true as const, preview }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
