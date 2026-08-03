"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { getSystemStatusSchema, SystemStatus } from "@/schemas/status"

// Reads the dependency lights (bot + Traveos). Open to any authenticated user,
// unlike getBotStatusAction next door, which hits the admin-only /bot proxy.
export async function getSystemStatusAction(): Promise<
  { error: string } | { status: SystemStatus }
> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/status`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })

    const json = await res.json()
    const parsed = getSystemStatusSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao consultar o status do sistema." }
    }

    return { status: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
