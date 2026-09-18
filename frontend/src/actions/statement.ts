"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"
import {
  azulBatchLinkResponseSchema,
  azulBookingLinkResponseSchema,
  azulConnectResponseSchema,
  azulStatementEntriesResponseSchema,
  azulStatementResponseSchema,
  getPointBatchesSchema,
  statementCommitResponseSchema,
  statementReconciliationResponseSchema,
  type AzulStatementEntries,
  type AzulStatementLinkedBatch,
  type AzulStatementLinkedIssuance,
  type AzulStatement,
  type StatementBatchSelection,
  type StatementReconciliation,
} from "@/schemas/inventory"
import { getIssuancesSchema } from "@/schemas/issuance"

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
    revalidatePath("/dashboard/accounts")
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// --- Statement page: entries and manual booking links ---

type EntriesResult =
  | { error: string }
  | { success: true; entries: AzulStatementEntries }

// Reads the synced lines for the statement page, each redemption already resolved to its
// issuance by locator. A never-synced account returns an empty window, not an error.
export async function getAzulStatementEntriesAction(
  accountId: string,
): Promise<EntriesResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/azul-statement/entries`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao carregar o extrato." }
    }
    const parsed = azulStatementEntriesResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, entries: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

type LinkResult =
  | { error: string }
  | { success: true; linked: AzulStatementLinkedIssuance | null }

// Pins a statement locator to an issuance (edit mode). Returns how the locator resolves now, so
// the page can update the line in place, and revalidates the statement page.
export async function linkAzulBookingAction(
  accountId: string,
  locator: string,
  issuanceId: string,
): Promise<LinkResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/azul-statement/links`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ locator, issuance_id: issuanceId }),
        cache: "no-store",
      },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao salvar o vínculo." }
    }
    const parsed = azulBookingLinkResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    revalidatePath(`/dashboard/accounts/${accountId}/statement`)
    return { success: true, linked: parsed.data.data.linked_issuance ?? null }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Removes a manual pin, handing the locator back to the automatic match.
export async function unlinkAzulBookingAction(
  accountId: string,
  locator: string,
): Promise<LinkResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/azul-statement/links`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ locator }),
        cache: "no-store",
      },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao remover o vínculo." }
    }
    const parsed = azulBookingLinkResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    revalidatePath(`/dashboard/accounts/${accountId}/statement`)
    return { success: true, linked: parsed.data.data.linked_issuance ?? null }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

type BatchLinkResult =
  | { error: string }
  | { success: true; linked: AzulStatementLinkedBatch | null }

// Pins a purchase/accrual line to a batch. occurredOn/points let the response recompute the auto
// match after an unlink; both link and unlink revalidate the statement page.
export async function linkAzulBatchAction(
  accountId: string,
  key: string,
  batchId: string,
  occurredOn: string,
  points: number,
): Promise<BatchLinkResult> {
  return batchLinkRequest("POST", accountId, {
    key,
    batch_id: batchId,
    occurred_on: occurredOn,
    points,
  })
}

export async function unlinkAzulBatchAction(
  accountId: string,
  key: string,
  occurredOn: string,
  points: number,
): Promise<BatchLinkResult> {
  return batchLinkRequest("DELETE", accountId, { key, occurred_on: occurredOn, points })
}

async function batchLinkRequest(
  method: "POST" | "DELETE",
  accountId: string,
  body: Record<string, unknown>,
): Promise<BatchLinkResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/azul-statement/batch-links`,
      {
        method,
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify(body),
        cache: "no-store",
      },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao salvar o vínculo." }
    }
    const parsed = azulBatchLinkResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    revalidatePath(`/dashboard/accounts/${accountId}/statement`)
    return { success: true, linked: parsed.data.data.linked_batch ?? null }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// One batch option for the manual-link picker.
export type BatchLinkOption = {
  id: string
  origin: string
  acquired_on: string
  points: number
  cost_cents: number
}

// Lists the account's batches for the batch-link picker. There are few per account, so the whole
// list comes back and the picker filters it client-side.
export async function getAccountBatchesAction(
  accountId: string,
): Promise<{ error: string } | { success: true; batches: BatchLinkOption[] }> {
  try {
    const cookieStore = await cookies()
    const token = cookieStore.get("auth_token")?.value
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/point-batches?account_id=${encodeURIComponent(accountId)}`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao buscar os lotes." }
    }
    const parsed = getPointBatchesSchema.response.safeParse(json)
    if (!parsed.success) {
      return { error: "Resposta inesperada do servidor." }
    }
    const batches: BatchLinkOption[] = (parsed.data.data ?? []).map((batch) => ({
      id: batch.id,
      origin: batch.origin?.name ?? "Lote",
      acquired_on: batch.acquired_on,
      points: batch.points,
      cost_cents: batch.cost_cents,
    }))
    return { success: true, batches }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Assigns the IATA code for a statement place name. Global (applies to every account's statement),
// so it revalidates the current statement page and the caller refreshes to pick the mapping up on
// every line.
export async function assignAzulCityCodeAction(
  accountId: string,
  city: string,
  iata: string,
): Promise<{ error: string } | { success: true }> {
  const code = iata.trim().toUpperCase()
  if (!/^[A-Za-z]{3}$/.test(code)) {
    return { error: "O código IATA precisa ter 3 letras." }
  }
  try {
    const cookieStore = await cookies()
    const token = cookieStore.get("auth_token")?.value
    const res = await fetch(`${env.BACKEND_URL}/api/v1/azul-city-codes`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ city: city.trim(), iata: code }),
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao salvar o código." }
    }
    revalidatePath(`/dashboard/accounts/${accountId}/statement`)
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// One issuance option for the manual-link picker: the fields it shows and the id it sends back.
export type IssuanceLinkOption = {
  id: string
  program_locator: string
  main_passenger_name: string | null
  origin_iata_code: string | null
  destination_iata_code: string | null
  points: number
  issued_on: string
}

// Searches the account's issuances by locator (reusing GET /issuances?account_id=&q=), for the
// manual-link picker. An empty query returns nothing rather than the whole account.
export async function searchAccountIssuancesAction(
  accountId: string,
  query: string,
): Promise<{ error: string } | { success: true; issuances: IssuanceLinkOption[] }> {
  const trimmed = query.trim()
  if (trimmed === "") {
    return { success: true, issuances: [] }
  }
  try {
    const cookieStore = await cookies()
    const token = cookieStore.get("auth_token")?.value
    const params = new URLSearchParams({
      account_id: accountId,
      q: trimmed,
      limit: "10",
    })
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances?${params.toString()}`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao buscar emissões." }
    }
    const parsed = getIssuancesSchema.response.safeParse(json)
    if (!parsed.success) {
      return { error: "Resposta inesperada do servidor." }
    }
    const issuances: IssuanceLinkOption[] = (parsed.data.data ?? []).map((issuance) => ({
      id: issuance.id,
      program_locator: issuance.program_locator,
      main_passenger_name: issuance.main_passenger_name,
      origin_iata_code: issuance.origin_iata_code,
      destination_iata_code: issuance.destination_iata_code,
      points: issuance.points,
      issued_on: issuance.issued_on,
    }))
    return { success: true, issuances }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
