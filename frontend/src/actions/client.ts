"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { createClientSchema, updateClientSchema } from "@/schemas/client"
import { z } from "zod"

export async function createClientAction(
  data: z.infer<typeof createClientSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/clients`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        name: data.name,
        phone: data.phone || undefined,
        agency_id: data.agency_id,
      }),
    })

    const json = await res.json()
    const parsed = createClientSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar cliente." }
    }

    // The created client is echoed back so callers that need to act on it right away -
    // the sales combobox links it to the sale the moment it is created - don't have to
    // refetch the whole list to find out its id.
    const client = parsed.data?.data
    if (!client) {
      return { error: "Erro ao cadastrar cliente." }
    }

    return { success: true as const, client }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateClientAction(
  clientId: string,
  data: z.infer<typeof updateClientSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/clients/${clientId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        name: data.name,
        phone: data.phone || undefined,
        agency_id: data.agency_id,
      }),
    })

    const json = await res.json()
    const parsed = updateClientSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar cliente." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteClientAction(clientId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/clients/${clientId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    })

    if (!res.ok) {
      // 422 is a client that still has payments: the backend says so in words, which
      // tells the operator what to undo first. Anything else stays generic.
      if (res.status === 422) {
        const json = (await res.json().catch(() => null)) as { message?: string } | null
        return { error: json?.message || "Erro ao deletar cliente." }
      }
      return { error: "Erro ao deletar cliente." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
