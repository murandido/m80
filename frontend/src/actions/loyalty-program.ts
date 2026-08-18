"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { createLoyaltyProgramSchema, updateLoyaltyProgramSchema } from "@/schemas/loyalty-program"
import { z } from "zod"

// The form uses "" for "this program has no researched cap", which is not something the
// API can read: cpf_limit is a number there, and the two enums reject an empty string. So
// the empty ones become explicit nulls, which is how the API stores "no cap".
function withNullableCaps<T extends Record<string, unknown>>(data: T) {
  const normalized: Record<string, unknown> = { ...data }
  for (const key of ["cpf_limit", "cpf_limit_window", "cpf_limit_mode"]) {
    if (normalized[key] === "" || normalized[key] === undefined) normalized[key] = null
  }
  // The limit travels as text through the form; the API takes a number.
  if (typeof normalized.cpf_limit === "string") {
    normalized.cpf_limit = Number(normalized.cpf_limit)
  }
  return normalized
}

export async function createLoyaltyProgramAction(
  data: z.infer<typeof createLoyaltyProgramSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/loyalty-programs`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(withNullableCaps(data)),
    })

    const json = await res.json()
    const parsed = createLoyaltyProgramSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar programa de fidelidade." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateLoyaltyProgramAction(
  loyaltyProgramId: string,
  data: z.infer<typeof updateLoyaltyProgramSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/loyalty-programs/${loyaltyProgramId}`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(withNullableCaps(data)),
      },
    )

    const json = await res.json()
    const parsed = updateLoyaltyProgramSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar programa de fidelidade." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteLoyaltyProgramAction(loyaltyProgramId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/loyalty-programs/${loyaltyProgramId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    if (!res.ok) {
      return { error: "Erro ao deletar programa de fidelidade." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
