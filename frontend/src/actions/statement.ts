"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"
import {
  azulConnectResponseSchema,
  azulStatementResponseSchema,
  statementCommitResponseSchema,
  statementReconciliationResponseSchema,
  type AzulStatement,
  type StatementBatchSelection,
  type StatementReconciliation,
} from "@/schemas/inventory"

// Server actions for the statement reconciliation. They forward the operator's file to the
// Go API with the JWT attached, which is the panel's standard mutation path.
//
// The file is uploaded again on every call rather than parked server-side. It is a few
// kilobytes, and nothing about a statement is stored - the whole feature leaves no
// temporary state behind, which is what makes re-submitting the same file harmless.
async function postStatement(path: string, body: FormData) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  return fetch(`${env.BACKEND_URL}/api/v1/point-batches/statement/${path}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body,
    cache: "no-store",
  })
}

// Explicit result types rather than inferred ones: these cross the server/client boundary,
// and an inferred union of object literals reaches the caller with every key optional,
// which defeats the narrowing the caller does on "error".
type PreviewResult =
  | { error: string }
  | { success: true; reconciliation: StatementReconciliation }

type CommitResult =
  | { error: string }
  | { success: true; created: number; credits: number; skipped: number }

export async function previewStatementAction(formData: FormData): Promise<PreviewResult> {
  const file = formData.get("file")
  if (!(file instanceof File) || file.size === 0) {
    return { error: "Escolha o arquivo do extrato antes de continuar." }
  }

  try {
    const res = await postStatement("preview", formData)
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao conciliar o extrato." }
    }

    const parsed = statementReconciliationResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, reconciliation: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function commitStatementAction(formData: FormData): Promise<CommitResult> {
  const file = formData.get("file")
  if (!(file instanceof File) || file.size === 0) {
    return { error: "Escolha o arquivo do extrato antes de continuar." }
  }

  try {
    const res = await postStatement("commit", formData)
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao lançar os lotes." }
    }

    const parsed = statementCommitResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }

    revalidatePath("/dashboard/sales/inventory")
    return {
      success: true,
      created: parsed.data.data.created,
      credits: parsed.data.data.credits,
      skipped: parsed.data.data.skipped,
    }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// --- Azul: sync state, reconciliation and the one-time connect ---
//
// The Azul statement is not uploaded - the collector syncs it into the database - so these
// routes are account-scoped and carry no file. They live under /accounts/{id} rather than under
// /point-batches/statement, which stays Smiles-only.

async function accountsAuth() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  return { base: `${env.BACKEND_URL}/api/v1/accounts`, token }
}

type AzulStatementResult =
  | { error: string }
  | { success: true; statement: AzulStatement }

// Reads the synced window and the sync-state badge in one call. An account that has never been
// synced still returns a reconciliation (empty) alongside a pending or unconnected state.
export async function getAzulStatementAction(
  accountId: string,
): Promise<AzulStatementResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/azul-statement`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao ler o extrato sincronizado." }
    }
    const parsed = azulStatementResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, statement: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Records the chosen credits as batches from the synced window. Unlike the Smiles commit there
// is no file to re-send: the statement is re-read from the database, so only the selection
// travels, as a JSON body.
export async function commitAzulStatementAction(
  accountId: string,
  batches: StatementBatchSelection[],
): Promise<CommitResult> {
  if (batches.length === 0) {
    return { error: "Marque ao menos um crédito." }
  }
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/azul-statement/commit`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ batches }),
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao lançar os lotes." }
    }
    const parsed = statementCommitResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    revalidatePath("/dashboard/sales/inventory")
    return {
      success: true,
      created: parsed.data.data.created,
      credits: parsed.data.data.credits,
      skipped: parsed.data.data.skipped,
    }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Starts the one-time login that discovers and stores the account's customerKey. The login takes
// minutes and runs in the background on the server, so this returns as soon as it is accepted
// (HTTP 202); the panel then polls the sync-state badge (connecting -> connected, or an error
// state) for the outcome. Everything after the connect is the read-only sync on the ticker.
export async function connectAzulAccountAction(
  accountId: string,
): Promise<{ error: string } | { success: true }> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/azul-connect`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao conectar a conta." }
    }
    const parsed = azulConnectResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    revalidatePath("/dashboard/sales/inventory")
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
