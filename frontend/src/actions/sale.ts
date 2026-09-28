"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  createClientPaymentSchema,
  getClientPaymentsSchema,
  undoClientPaymentSchema,
  updateSaleSchema,
} from "@/schemas/sale"
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

export async function createClientPaymentAction(
  data: z.infer<typeof createClientPaymentSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/client-payments`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    })

    const parsed = createClientPaymentSchema.response.safeParse(await res.json())
    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao registrar o pagamento." }
    }

    const result = parsed.data?.data
    if (!result) {
      return { error: "Erro ao registrar o pagamento." }
    }

    return { success: true as const, result }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Fetched on demand when the history is opened, not with the page: it is one client's
// history, and the page is a period of every client's sales.
export async function getClientPaymentsAction(clientId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/sales/client-payments?client_id=${encodeURIComponent(clientId)}`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )

    const parsed = getClientPaymentsSchema.response.safeParse(await res.json())
    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao buscar os pagamentos do cliente." }
    }

    const history = parsed.data?.data
    if (!history) {
      return { error: "Erro ao buscar os pagamentos do cliente." }
    }

    return { success: true as const, history }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function undoClientPaymentAction(clientPaymentId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/sales/client-payments/${clientPaymentId}`,
      { method: "DELETE", headers: { Authorization: `Bearer ${token}` } },
    )

    const parsed = undoClientPaymentSchema.response.safeParse(await res.json())
    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao desfazer o pagamento." }
    }

    const result = parsed.data?.data
    if (!result) {
      return { error: "Erro ao desfazer o pagamento." }
    }

    return { success: true as const, result }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
