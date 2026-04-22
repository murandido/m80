"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { createUserSchema } from "@/schemas/user"
import { z } from "zod"

export async function createUserAction(
  data: z.infer<typeof createUserSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/users`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = createUserSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar usuário." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
