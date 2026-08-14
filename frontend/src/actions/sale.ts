"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  createSalePaymentSchema,
  getSalePaymentsSchema,
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

// The statement of a sale: what the client has handed over, in parts. A sale is settled
// when these add up to its value, which is why there is no "mark as paid" action any more.
export async function getSalePaymentsAction(saleId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/sales/${saleId}/payments`,
      {
        headers: { Authorization: `Bearer ${token}` },
        cache: "no-store",
      },
    )

    const parsed = getSalePaymentsSchema.response.safeParse(await res.json())
    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao buscar os lançamentos." }
    }

    return { success: true as const, payments: parsed.data?.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function createSalePaymentAction(
  saleId: string,
  data: z.infer<typeof createSalePaymentSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/sales/${saleId}/payments`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(data),
      },
    )

    const parsed = createSalePaymentSchema.response.safeParse(await res.json())
    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao lançar o pagamento." }
    }

    const payment = parsed.data?.data
    if (!payment) {
      return { error: "Erro ao lançar o pagamento." }
    }

    return { success: true as const, payment }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteSalePaymentAction(
  saleId: string,
  paymentId: string,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/sales/${saleId}/payments/${paymentId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    if (!res.ok) {
      return { error: "Erro ao apagar o lançamento." }
    }

    return { success: true as const }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
