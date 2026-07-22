"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  createIssuanceSchema,
  updateIssuanceSchema,
  deleteIssuanceSchema,
  cancelIssuanceSchema,
  uncancelIssuanceSchema,
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
