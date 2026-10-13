"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  cancelIssuanceSchema,
  createIssuanceSchema,
  deleteIssuanceSchema,
  issuanceAdjustmentRequestSchema,
  issuanceAdjustmentResponseSchema,
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

// Records an alteration (adjustmentId null) or rewrites one already recorded. The body is
// the same either way: an alteration is always sent whole.
export async function saveIssuanceAdjustmentAction(
  issuanceId: string,
  adjustmentId: string | null,
  data: z.infer<typeof issuanceAdjustmentRequestSchema>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const path = adjustmentId
    ? `${issuanceId}/adjustments/${adjustmentId}`
    : `${issuanceId}/adjustments`

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${path}`, {
      method: adjustmentId ? "PATCH" : "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = issuanceAdjustmentResponseSchema.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao salvar a alteração." }
    }

    const adjustment = parsed.data?.data
    if (!adjustment) {
      return { error: "Erro ao salvar a alteração." }
    }
    return { success: true as const, adjustment }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteIssuanceAdjustmentAction(issuanceId: string, adjustmentId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/issuances/${issuanceId}/adjustments/${adjustmentId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    const json = await res.json()
    const parsed = deleteIssuanceSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao excluir a alteração." }
    }
    return { success: true as const }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
