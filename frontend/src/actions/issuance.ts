"use server"

import { cookies } from "next/headers"
import { readActionError, readBackendJson } from "@/lib/backend-response"
import { env } from "@/lib/env"
import {
  cancelIssuanceSchema,
  createIssuanceSchema,
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao criar emissão.") }
    }

    const parsed = createIssuanceSchema.response.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao salvar emissão.") }
    }

    const parsed = updateIssuanceSchema.response.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao cancelar emissão.") }
    }

    const parsed = cancelIssuanceSchema.response.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao reverter cancelamento.") }
    }

    const parsed = uncancelIssuanceSchema.response.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      // The backend's message is deliberately vague for the common failure, because
      // Traveos answers almost every one of them identically. Pass it through rather
      // than dressing it up - including on its 502/503, which are worded for the operator.
      return {
        error: await readActionError(res, "Erro ao puxar dados da Traveos.", {
          keepServerErrorMessage: true,
        }),
      }
    }

    const parsed = syncIssuanceWithTraveosSchema.response.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao deletar emissão.") }
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao salvar a alteração.") }
    }

    const parsed = issuanceAdjustmentResponseSchema.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao excluir a alteração.") }
    }

    return { success: true as const }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
