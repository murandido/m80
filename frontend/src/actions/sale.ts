"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { updateSaleSchema } from "@/schemas/sale"
import { z } from "zod"

export async function updateSaleAction(
  saleId: string,
  data: z.infer<typeof updateSaleSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/${saleId}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      // Serialized as-is: an explicit `client_id: null` has to reach the backend to
      // unlink the client, so the key must survive JSON.stringify.
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = updateSaleSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar venda." }
    }

    const sale = parsed.data?.data
    if (!sale) {
      return { error: "Erro ao atualizar venda." }
    }

    return { success: true as const, sale }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
