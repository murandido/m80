"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"

// Server actions for linking a LATAM account, which is read for its balance only.
//
// The login happens on LATAM's own page in the operator's browser; these actions only hand out
// that page and, later, pass LATAM's answer on to the API (see @/carriers/latam).

async function backendAuth() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  return { base: `${env.BACKEND_URL}/api/v1`, token }
}

export async function startLatamLoginAction(
  accountId: string,
): Promise<{ error: string } | { success: true; authorizeUrl: string }> {
  try {
    const { base, token } = await backendAuth()
    const res = await fetch(`${base}/accounts/${encodeURIComponent(accountId)}/latam-connect`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    const authorizeUrl = json?.data?.authorize_url
    if (!res.ok || typeof authorizeUrl !== "string") {
      return { error: json?.message || "Erro ao preparar o login da LATAM." }
    }
    return { success: true, authorizeUrl }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function finishLatamLoginAction(
  callbackUrl: string,
): Promise<{ error: string } | { success: true; accountId: string }> {
  try {
    const { base, token } = await backendAuth()
    const res = await fetch(`${base}/latam-connect/callback`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ callback_url: callbackUrl }),
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    const accountId = json?.data?.account_id
    if (!res.ok || typeof accountId !== "string") {
      return { error: json?.message || "Erro ao concluir a conexão com a LATAM." }
    }
    revalidatePath("/dashboard/accounts")
    return { success: true, accountId }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function disconnectLatamAccountAction(
  accountId: string,
): Promise<{ error: string } | { success: true }> {
  try {
    const { base, token } = await backendAuth()
    const res = await fetch(`${base}/accounts/${encodeURIComponent(accountId)}/latam-connect`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao desconectar a conta." }
    }
    revalidatePath("/dashboard/accounts")
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
