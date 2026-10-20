"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { createMcpTokenSchema, revokeMcpTokenSchema } from "@/schemas/mcp-token"
import { z } from "zod"

// Explicit return type so `"error" in result` narrows at the call site (see the note on
// checkAgencyNicknameAvailabilityAction).
export async function createMcpTokenAction(
  data: z.infer<typeof createMcpTokenSchema.request>,
): Promise<{ error: string } | { token: string }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const request = createMcpTokenSchema.request.safeParse(data)
  if (!request.success) {
    return { error: request.error.issues[0]?.message ?? "Nome inválido." }
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/mcp-tokens`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(request.data),
    })

    const json = await res.json().catch(() => null)
    const parsed = createMcpTokenSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao criar token." }
    }

    const created = parsed.data?.data?.token
    if (!created) {
      return { error: "Erro ao criar token." }
    }

    return { token: created }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function revokeMcpTokenAction(
  tokenId: string,
): Promise<{ error: string } | { success: true }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/mcp-tokens/${encodeURIComponent(tokenId)}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    const json = await res.json().catch(() => null)
    const parsed = revokeMcpTokenSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao revogar token." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
