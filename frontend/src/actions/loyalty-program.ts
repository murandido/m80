"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { createLoyaltyProgramSchema, updateLoyaltyProgramSchema } from "@/schemas/loyalty-program"
import { z } from "zod"

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
      body: JSON.stringify(data),
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
        body: JSON.stringify(data),
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
