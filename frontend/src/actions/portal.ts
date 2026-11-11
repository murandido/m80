"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"
import { getPortalAccountsSchema, type PortalAccount } from "@/schemas/portal"
import { statementEntriesResponseSchema, type StatementEntries } from "@/schemas/inventory"

// Server actions for the statement client's portal. Every call goes to /api/v1/portal, which
// checks the role and the account grant itself; nothing here decides access.

async function portalAuth() {
  const token = (await cookies()).get("auth_token")?.value
  return { base: `${env.BACKEND_URL}/api/v1/portal/accounts`, token }
}

export async function getPortalAccountsAction(): Promise<
  { error: string } | { success: true; accounts: PortalAccount[] }
> {
  try {
    const { base, token } = await portalAuth()
    const res = await fetch(base, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) return { error: json?.message || "Erro ao carregar as contas." }
    const parsed = getPortalAccountsSchema.response.safeParse(json)
    if (!parsed.success) return { error: "Resposta inesperada do servidor." }
    return { success: true, accounts: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function getPortalEntriesAction(
  accountId: string,
): Promise<{ error: string } | { success: true; entries: StatementEntries }> {
  try {
    const { base, token } = await portalAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/entries`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) return { error: json?.message || "Erro ao carregar o extrato." }
    const parsed = statementEntriesResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) return { error: "Resposta inesperada do servidor." }
    return { success: true, entries: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Saves a new airline password. It does not connect on its own: the client presses Conectar
// (Azul) or asks the team for the code step (Smiles) afterwards.
export async function updatePortalPasswordAction(
  accountId: string,
  password: string,
): Promise<{ error: string } | { success: true }> {
  try {
    const { base, token } = await portalAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/password`, {
      method: "PUT",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) return { error: json?.message || "Erro ao salvar a senha." }
    revalidatePath("/portal")
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Starts the Azul login in the background (HTTP 202); the page polls the status route for the
// outcome, like the staff screen.
export async function connectPortalAzulAction(
  accountId: string,
): Promise<{ error: string } | { success: true }> {
  try {
    const { base, token } = await portalAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/azul-connect`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) return { error: json?.message || "Erro ao conectar a conta." }
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
