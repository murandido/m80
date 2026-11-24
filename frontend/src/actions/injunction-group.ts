"use server"

import { cookies } from "next/headers"
import type { z } from "zod"
import { env } from "@/lib/env"
import { injunctionGroupFormSchema, schemas } from "@/schemas/injunction-group"
import type { InjunctionAccountOption, InjunctionGroupForm } from "@/schemas/injunction-group"
import { getAgenciesSchema } from "@/schemas/agency"
import { getUsersSchema } from "@/schemas/user"

type Envelope = z.ZodType<{ success: boolean; message: string; data?: unknown }>

async function callApi<S extends Envelope>(
  path: string,
  schema: S,
  errorMessage: string,
  init: RequestInit = {},
): Promise<{ error: string } | { data: z.infer<S>["data"] }> {
  const token = (await cookies()).get("auth_token")?.value
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1${path}`, {
      ...init,
      headers: { ...init.headers, Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      cache: "no-store",
    })
    const parsed = schema.safeParse(await res.json())
    if (!res.ok || !parsed.success) {
      return { error: (parsed.success && parsed.data.message) || errorMessage }
    }
    return { data: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function getMicrosoftConnectionAction() {
  return callApi("/microsoft-connection", schemas.connection, "Erro ao ler a conexão Microsoft.")
}

export async function startMicrosoftConnectionAction() {
  return callApi("/microsoft-connection/start", schemas.start, "Erro ao iniciar o login Microsoft.", { method: "POST" })
}

export async function disconnectMicrosoftAction() {
  return callApi("/microsoft-connection", schemas.connection, "Erro ao desconectar.", { method: "DELETE" })
}

export async function listInjunctionGroupsAction() {
  return callApi("/injunction-groups", schemas.groups, "Erro ao listar Tropas.")
}

export async function saveInjunctionGroupAction(id: string | null, input: InjunctionGroupForm) {
  const parsed = injunctionGroupFormSchema.safeParse(input)
  if (!parsed.success) return { error: parsed.error.issues[0]?.message ?? "Dados inválidos." }
  const form = parsed.data
  const body = {
    name: form.name,
    agency_id: form.agency_id,
    whatsapp_chat_name: form.whatsapp_chat_name || null,
    // null keeps the stored link on an update; a new one replaces it.
    spreadsheet_url: form.spreadsheet_url ? form.spreadsheet_url : id ? null : "",
    accounts: form.accounts,
    issuers: form.issuers,
  }
  return callApi(id ? `/injunction-groups/${encodeURIComponent(id)}` : "/injunction-groups", schemas.group, "Erro ao salvar a Tropa.", {
    method: id ? "PUT" : "POST",
    body: JSON.stringify(body),
  })
}

export async function deleteInjunctionGroupAction(id: string) {
  return callApi(`/injunction-groups/${encodeURIComponent(id)}`, schemas.empty, "Erro ao excluir a Tropa.", { method: "DELETE" })
}

export async function checkInjunctionGroupAction(id: string, from: string, to: string) {
  const query = new URLSearchParams({ from, to }).toString()
  return callApi(`/injunction-groups/${encodeURIComponent(id)}/check?${query}`, schemas.check, "Erro ao conferir a Tropa.")
}

export async function importInjunctionPeriodAction(id: string, from: string, to: string) {
  return callApi(`/injunction-groups/${encodeURIComponent(id)}/import-period`, schemas.importPeriod, "Erro ao importar o período.", {
    method: "POST",
    body: JSON.stringify({ from, to }),
  })
}

// What the form's pickers need. Accounts come from GET /accounts, which returns passwords for
// the staff who log into the programs: they are reduced here, on the server, to an id and a
// label before anything reaches the browser.
export async function injunctionFormOptionsAction(): Promise<{
  accounts: InjunctionAccountOption[]
  users: { id: string; name: string }[]
  agencies: { id: string; name: string }[]
}> {
  const token = (await cookies()).get("auth_token")?.value
  const headers = { Authorization: `Bearer ${token}` }
  const get = async (path: string) => {
    try {
      const res = await fetch(`${env.BACKEND_URL}/api/v1${path}`, { headers, cache: "no-store" })
      return res.ok ? await res.json() : null
    } catch {
      return null
    }
  }
  const [accountsRaw, usersRaw, agenciesRaw] = await Promise.all([
    get("/accounts?category=injunction"),
    get("/users"),
    get("/agencies"),
  ])

  type RawAccount = {
    id: string
    loyalty_program?: { name?: string; code?: string }
    nicknames?: { nickname: string }[]
  }
  const accounts: InjunctionAccountOption[] = ((accountsRaw?.data ?? []) as RawAccount[]).map((account) => {
    const nicknames = (account.nicknames ?? []).map((n) => n.nickname).join(", ")
    const program = account.loyalty_program?.name ?? account.loyalty_program?.code ?? ""
    return { id: account.id, label: `${program} · ${nicknames || account.id.slice(0, 8)}` }
  })

  const users = getUsersSchema.response.safeParse(usersRaw)
  const agencies = getAgenciesSchema.response.safeParse(agenciesRaw)
  return {
    accounts,
    users: users.success
      ? (users.data.data ?? []).filter((u) => u.role !== "statement_client").map((u) => ({ id: u.id, name: u.name }))
      : [],
    agencies: agencies.success ? (agencies.data.data ?? []).map((a) => ({ id: a.id, name: a.name })) : [],
  }
}
