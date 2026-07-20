"use server"

import { cookies } from "next/headers"
import { z } from "zod"
import { env } from "@/lib/env"
import { createAirportSchema, updateAirportSchema } from "@/schemas/airport"

// Normalizes a create/update payload before sending it to the backend: IATA
// codes are stored uppercase, and blank optional fields are trimmed.
function toAirportPayload(data: z.infer<typeof createAirportSchema.request>) {
  return {
    iata_code: data.iata_code.trim().toUpperCase(),
    name: data.name.trim(),
    city: data.city.trim(),
    state: data.state.trim(),
    country: data.country.trim().toUpperCase(),
  }
}

export async function createAirportAction(
  data: z.infer<typeof createAirportSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/airports`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(toAirportPayload(data)),
    })

    const json = await res.json()
    const parsed = createAirportSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar aeroporto." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateAirportAction(
  airportId: string,
  data: z.infer<typeof updateAirportSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/airports/${airportId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(toAirportPayload(data)),
    })

    const json = await res.json()
    const parsed = updateAirportSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar aeroporto." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteAirportAction(airportId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/airports/${airportId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    })

    if (!res.ok) {
      const json = await res.json().catch(() => null)
      return { error: json?.message || "Erro ao deletar aeroporto." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
