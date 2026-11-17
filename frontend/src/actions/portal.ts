"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"
import {
  changeOwnPasswordSchema,
  getPortalAccountsSchema,
  type ChangeOwnPasswordRequest,
  type PortalAccount,
} from "@/schemas/portal"
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

// Changes the client's own M80 password. Only the password fields go: name and phone are what the
// team registered, and the phone is the login. A wrong current password is a 403 (never 401, so a
// typo does not log the client out).
export async function changeOwnPasswordAction(
  data: ChangeOwnPasswordRequest,
): Promise<{ error: string } | { success: true }> {
  const parsed = changeOwnPasswordSchema.safeParse(data)
  if (!parsed.success) return { error: parsed.error.issues[0]?.message ?? "Dados inválidos." }
  try {
    const token = (await cookies()).get("auth_token")?.value
    const res = await fetch(`${env.BACKEND_URL}/api/v1/auth/me`, {
      method: "PUT",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({
        current_password: parsed.data.current_password,
        new_password: parsed.data.new_password,
      }),
      cache: "no-store",
    })
    if (res.status === 403) return { error: "Senha atual incorreta." }
    if (!res.ok) return { error: "Não consegui trocar a senha. Tente de novo." }
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
