"use server"

import { cookies } from "next/headers"
import { readActionError, readBackendJson } from "@/lib/backend-response"
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao atualizar venda.") }
    }

    const parsed = updateSaleSchema.response.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao registrar o pagamento.") }
    }

    const parsed = createClientPaymentSchema.response.safeParse(await readBackendJson(res))
    const result = parsed.data?.data
    if (!result) {
      return { error: "Erro ao registrar o pagamento." }
    }

    return { success: true as const, result }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Which history to open: one client's whole history, or - with no single client - the
// payments of the agency's clients (or of the filtered ones) received in the period.
export type ClientPaymentsQuery =
  | { clientId: string }
  | { clientIds: string[]; agencyId: string; dateFrom: string; dateTo: string }

// Fetched on demand when the history is opened, not with the page: the page is a period of
// sales, and the history is a different list that nobody needs until they ask for it.
export async function getClientPaymentsAction(query: ClientPaymentsQuery) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const params = new URLSearchParams()
  if ("clientId" in query) {
    params.set("client_id", query.clientId)
  } else {
    if (query.clientIds.length > 0) params.set("client_ids", query.clientIds.join(","))
    if (query.agencyId) params.set("agency_id", query.agencyId)
    if (query.dateFrom) params.set("date_from", query.dateFrom)
    if (query.dateTo) params.set("date_to", query.dateTo)
  }

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/sales/client-payments?${params.toString()}`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao buscar os pagamentos do cliente.") }
    }

    const parsed = getClientPaymentsSchema.response.safeParse(await readBackendJson(res))
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

    if (!res.ok) {
      return { error: await readActionError(res, "Erro ao desfazer o pagamento.") }
    }

    const parsed = undoClientPaymentSchema.response.safeParse(await readBackendJson(res))
    const result = parsed.data?.data
    if (!result) {
      return { error: "Erro ao desfazer o pagamento." }
    }

    return { success: true as const, result }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
