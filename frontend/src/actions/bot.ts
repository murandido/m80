"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  BotPairMethod,
  BotStatus,
  cancelBotPairingSchema,
  getBotStatusSchema,
  logoutBotSchema,
  startBotPairingSchema,
} from "@/schemas/bot"

export async function getBotStatusAction(): Promise<
  { error: string } | { status: BotStatus }
> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/status`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })

    const json = await res.json()
    const parsed = getBotStatusSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao consultar status do bot." }
    }

    return { status: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function startPairingAction(
  method: BotPairMethod,
  phone?: string,
): Promise<{ error: string } | { status: BotStatus }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/pair`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ method, phone }),
    })

    const json = await res.json()
    const parsed = startBotPairingSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao iniciar pareamento." }
    }

    return { status: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function cancelPairingAction(): Promise<
  { error: string } | { status: BotStatus }
> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/pair/cancel`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    })

    const json = await res.json()
    const parsed = cancelBotPairingSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao cancelar pareamento." }
    }

    return { status: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function logoutBotAction(): Promise<
  { error: string } | { status: BotStatus }
> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/bot/logout`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    })

    const json = await res.json()
    const parsed = logoutBotSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao desconectar o bot." }
    }

    return { status: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
