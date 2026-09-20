"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  cancelIssuanceSchema,
  createIssuanceSchema,
  deleteIssuanceSchema,
  lookupTraveosSchema,
  syncIssuanceWithTraveosSchema,
  uncancelIssuanceSchema,
  updateIssuanceSchema,
} from "@/schemas/issuance"
import { z } from "zod"

export async function createIssuanceAction(
  data: z.infer<typeof createIssuanceSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = createIssuanceSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao criar emissão." }
    }

    const issuance = parsed.data?.data
    if (!issuance) {
      return { error: "Erro ao criar emissão." }
    }

    return { success: true as const, issuance }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateIssuanceAction(
  issuanceId: string,
  data: z.infer<typeof updateIssuanceSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceId}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = updateIssuanceSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao salvar emissão." }
    }

    return { success: true as const, issuance: parsed.data?.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function cancelIssuanceAction(
  issuanceId: string,
  data: z.infer<typeof cancelIssuanceSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceId}/cancel`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = cancelIssuanceSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cancelar emissão." }
    }

    return { success: true as const, issuance: parsed.data?.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function uncancelIssuanceAction(issuanceId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceId}/uncancel`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
    })

    const json = await res.json()
    const parsed = uncancelIssuanceSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao reverter cancelamento." }
    }

    return { success: true as const, issuance: parsed.data?.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function syncIssuanceWithTraveosAction(issuanceId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/issuances/${issuanceId}/traveos-sync`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        // Traveos drives the airline's own website, so issuing a ticket there takes
        // tens of seconds and the backend allows up to three minutes for the whole
        // sync. Next's default fetch has no timeout, which is what we want here -
        // but the cache must be off, or a repeat refetch could be served from it.
        cache: "no-store",
      },
    )

    const json = await res.json()
    const parsed = syncIssuanceWithTraveosSchema.response.safeParse(json)

    if (!res.ok) {
      // The backend's message is deliberately vague for the common failure, because
      // Traveos answers almost every one of them identically. Pass it through rather
      // than dressing it up.
      return { error: parsed.data?.message || "Erro ao puxar dados da Traveos." }
    }

    return { success: true as const, issuance: parsed.data?.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Looks a booking up at Traveos before the issuance exists, so the grid can fill the
// itinerary while the operator is still typing prices. Same timeout reasoning as the sync
// above: the vendor may issue the ticket first, which takes tens of seconds.
export async function lookupTraveosAction(data: z.infer<typeof lookupTraveosSchema.request>) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/traveos-lookup`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
      cache: "no-store",
    })

    const json = await res.json()
    const parsed = lookupTraveosSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao consultar a Traveos." }
    }

    return { success: true as const, lookup: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteIssuanceAction(issuanceId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    })

    const json = await res.json()
    const parsed = deleteIssuanceSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao deletar emissão." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
