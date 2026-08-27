"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  deactivateListenedChatSchema,
  decideListenedChatSchema,
  getMonitoredListenedChatsSchema,
  getPendingListenedChatsSchema,
  reactivateListenedChatSchema,
  setPDFModeSchema,
  setPromoModeSchema,
  stopListenedChatSchema,
} from "@/schemas/listened-chats"
import type { ListenedChat } from "@/schemas/listened-chats"

export async function getPendingListenedChatsAction(): Promise<
  { error: string } | { chats: ListenedChat[] }
> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/chats/pending`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })

    const json = await res.json()
    const parsed = getPendingListenedChatsSchema.response.safeParse(json)

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao listar chats pendentes." }
    }

    return { chats: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

async function decideListenedChatAction(
  id: string,
  decision: "approve" | "deny",
  errorMessage: string,
): Promise<{ error: string } | { chat: ListenedChat }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/bot/chats/${id}/${decision}`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    const json = await res.json()
    const parsed = decideListenedChatSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || errorMessage }
    }

    return { chat: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function approveListenedChatAction(id: string) {
  return decideListenedChatAction(id, "approve", "Erro ao aprovar chat.")
}

export async function denyListenedChatAction(id: string) {
  return decideListenedChatAction(id, "deny", "Erro ao negar chat.")
}

export async function getMonitoredListenedChatsAction(): Promise<
  { error: string } | { chats: ListenedChat[] }
> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/chats/active`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })

    const json = await res.json()
    const parsed = getMonitoredListenedChatsSchema.response.safeParse(json)

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao listar chats monitorados." }
    }

    return { chats: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

async function postChatAction(
  id: string,
  endpoint: string,
  schema: typeof stopListenedChatSchema,
  errorMessage: string,
): Promise<{ error: string } | { chat: ListenedChat }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/chats/${id}/${endpoint}`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    })

    const json = await res.json()
    const parsed = schema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || errorMessage }
    }

    return { chat: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function stopListenedChatAction(id: string) {
  return postChatAction(id, "stop", stopListenedChatSchema, "Erro ao parar de monitorar o chat.")
}

export async function deactivateListenedChatAction(id: string) {
  return postChatAction(id, "deactivate", deactivateListenedChatSchema, "Erro ao desativar o chat.")
}

export async function reactivateListenedChatAction(id: string) {
  return postChatAction(id, "reactivate", reactivateListenedChatSchema, "Erro ao reativar o chat.")
}

// Explicit on/off rather than a toggle, because the !pdf command writes the same
// column: a toggle here would flip whatever someone had just set from WhatsApp.
export async function setListenedChatPDFModeAction(
  id: string,
  enabled: boolean,
): Promise<{ error: string } | { chat: ListenedChat }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/chats/${id}/pdf-mode`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ enabled }),
    })

    const json = await res.json()
    const parsed = setPDFModeSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao salvar o modo PDF." }
    }

    return { chat: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Same explicit on/off as the PDF mode above, and for the same reason: the bot's
// !promo command writes this column too.
export async function setListenedChatPromoModeAction(
  id: string,
  enabled: boolean,
): Promise<{ error: string } | { chat: ListenedChat }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/chats/${id}/promo-mode`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ enabled }),
    })

    const json = await res.json()
    const parsed = setPromoModeSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao salvar o modo promoção." }
    }

    return { chat: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
