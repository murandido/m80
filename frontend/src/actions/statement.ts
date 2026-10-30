"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"
import {
  batchLinkResponseSchema,
  bookingLinkResponseSchema,
  azulConnectResponseSchema,
  statementEntriesResponseSchema,
  smilesBrowserChallengeResponseSchema,
  smilesChallengeResponseSchema,
  smilesConnectResponseSchema,
  syncedStatementResponseSchema,
  getPointBatchesSchema,
  statementCommitResponseSchema,
  type StatementEntries,
  type StatementLinkedBatch,
  type StatementLinkedIssuance,
  type SmilesAuthenticator,
  type SyncedProgram,
  type SyncedStatement,
  type StatementBatchSelection,
} from "@/schemas/inventory"
import { getIssuancesSchema } from "@/schemas/issuance"

// Server actions for the statement reconciliation.
//
// Both programmes' statements are synced into the database and read from there, so nothing here
// uploads a file any more: an action names an account and, where it matters, which programme's
// routes to talk to.

type CommitResult =
  | { error: string }
  | { success: true; created: number; credits: number; skipped: number }

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
  | { success: true; statement: SyncedStatement }

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
    const parsed = syncedStatementResponseSchema.safeParse(json)
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
  | { success: true; entries: StatementEntries }

// Reads the synced lines for the statement page, each redemption already resolved to its
// issuance by locator. A never-synced account returns an empty window, not an error.
export async function getStatementEntriesAction(
  program: SyncedProgram,
  accountId: string,
): Promise<EntriesResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/${program}-statement/entries`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao carregar o extrato." }
    }
    const parsed = statementEntriesResponseSchema.safeParse(json)
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
  | { success: true; linked: StatementLinkedIssuance | null }

// Pins a statement locator to an issuance (edit mode). Returns how the locator resolves now, so
// the page can update the line in place, and revalidates the statement page.
export async function linkStatementBookingAction(
  program: SyncedProgram,
  accountId: string,
  locator: string,
  issuanceId: string,
): Promise<LinkResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/${program}-statement/links`,
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
    const parsed = bookingLinkResponseSchema.safeParse(json)
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
export async function unlinkStatementBookingAction(
  program: SyncedProgram,
  accountId: string,
  locator: string,
): Promise<LinkResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/${program}-statement/links`,
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
    const parsed = bookingLinkResponseSchema.safeParse(json)
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
  | { success: true; linked: StatementLinkedBatch | null }

// Pins a purchase/accrual line to a batch. occurredOn/points let the response recompute the auto
// match after an unlink; both link and unlink revalidate the statement page.
export async function linkStatementBatchAction(
  program: SyncedProgram,
  accountId: string,
  key: string,
  batchId: string,
  occurredOn: string,
  points: number,
): Promise<BatchLinkResult> {
  return batchLinkRequest("POST", program, accountId, {
    key,
    batch_id: batchId,
    occurred_on: occurredOn,
    points,
  })
}

export async function unlinkStatementBatchAction(
  program: SyncedProgram,
  accountId: string,
  key: string,
  occurredOn: string,
  points: number,
): Promise<BatchLinkResult> {
  return batchLinkRequest("DELETE", program, accountId, { key, occurred_on: occurredOn, points })
}

async function batchLinkRequest(
  method: "POST" | "DELETE",
  program: SyncedProgram,
  accountId: string,
  body: Record<string, unknown>,
): Promise<BatchLinkResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/${program}-statement/batch-links`,
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
    const parsed = batchLinkResponseSchema.safeParse(json)
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

// --- Smiles: sync state, reconciliation and the three-step connect ---
//
// The shapes are the Azul ones; only the path differs, because the panel reconciles both
// programmes through the same screens. What is genuinely different is the connect: it is a
// conversation with a person in the middle, so it is three calls instead of one.

// Reads the synced window and the badge in one call, exactly like the Azul one.
export async function getSmilesStatementAction(
  accountId: string,
): Promise<AzulStatementResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/smiles-statement`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao ler o extrato sincronizado." }
    }
    const parsed = syncedStatementResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, statement: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function commitSmilesStatementAction(
  accountId: string,
  batches: StatementBatchSelection[],
): Promise<CommitResult> {
  if (batches.length === 0) {
    return { error: "Marque ao menos um crédito." }
  }
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/smiles-statement/commit`, {
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

type SmilesLoginStart =
  | { error: string }
  | { success: true; connected: true }
  | { success: true; connected: false; authenticators: SmilesAuthenticator[] }

// Step one: log in with the credentials already stored on the account. Almost always Smiles
// answers "a code is required" and hands back the channels the holder registered; an account
// without a second factor is connected outright.
export async function startSmilesLoginAction(accountId: string): Promise<SmilesLoginStart> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/smiles-connect`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao conectar a conta." }
    }
    const parsed = smilesConnectResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    if (parsed.data.data.connected) {
      revalidatePath("/dashboard/accounts")
      revalidatePath("/dashboard/sales/inventory")
      return { success: true, connected: true }
    }
    return {
      success: true,
      connected: false,
      authenticators: parsed.data.data.authenticators ?? [],
    }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Step two: ask Smiles to send the code on the chosen channel. Every call costs a real SMS or
// e-mail and Smiles throttles resends per channel, so this only ever runs from a button.
export async function challengeSmilesLoginAction(
  accountId: string,
  authenticatorId: string,
): Promise<{ error: string } | { success: true; sentTo: string }> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/smiles-connect/challenge`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ authenticator_id: authenticatorId }),
        cache: "no-store",
      },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao pedir o código." }
    }
    const parsed = smilesChallengeResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, sentTo: parsed.data.data.sent_to }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// The browser path. The extension runs the one POST the edge refuses to HTTP clients, in the
// operator's own browser; these two calls bracket it. Only the tenant's short-lived token
// travels to the page - never the account's password.
export interface SmilesBrowserChallengeMaterial {
  clientId: string
  mfaToken: string
  authenticatorId: string
  sentTo: string
  channel: string
}

export async function browserChallengeSmilesLoginAction(
  accountId: string,
  authenticatorId: string,
): Promise<{ error: string } | ({ success: true } & SmilesBrowserChallengeMaterial)> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/smiles-connect/challenge/browser`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ authenticator_id: authenticatorId }),
        cache: "no-store",
      },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao preparar o código." }
    }
    const parsed = smilesBrowserChallengeResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    const data = parsed.data.data
    return {
      success: true,
      clientId: data.client_id,
      mfaToken: data.mfa_token,
      authenticatorId: data.authenticator_id,
      sentTo: data.sent_to,
      channel: data.channel,
    }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function completeBrowserChallengeSmilesLoginAction(
  accountId: string,
  authenticatorId: string,
  oobCode: string,
): Promise<{ error: string } | { success: true; sentTo: string }> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/smiles-connect/challenge/complete`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ authenticator_id: authenticatorId, oob_code: oobCode }),
        cache: "no-store",
      },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao confirmar o código." }
    }
    const parsed = smilesChallengeResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, sentTo: parsed.data.data.sent_to }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Step three: hand over the digits. A wrong code comes back as an error the dialog shows without
// closing, so the operator retypes it - nothing here asks for another code, because Smiles counts
// failed attempts and throttles the channel.
export async function confirmSmilesLoginAction(
  accountId: string,
  code: string,
): Promise<{ error: string } | { success: true }> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/smiles-connect/code`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao confirmar o código." }
    }
    revalidatePath("/dashboard/accounts")
    revalidatePath("/dashboard/sales/inventory")
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Forgets the stored session and the synced window. The batches already recorded from the
// statement are untouched - only the synced extract goes.
export async function disconnectSmilesAccountAction(
  accountId: string,
): Promise<{ error: string } | { success: true }> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(`${base}/${encodeURIComponent(accountId)}/smiles-connect`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao desconectar a conta." }
    }
    revalidatePath("/dashboard/accounts")
    revalidatePath("/dashboard/sales/inventory")
    revalidatePath(`/dashboard/accounts/${accountId}/statement`)
    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// The per-account statement page, for Smiles. Same shape as the Azul entries.
export async function getSmilesStatementEntriesAction(
  accountId: string,
): Promise<EntriesResult> {
  try {
    const { base, token } = await accountsAuth()
    const res = await fetch(
      `${base}/${encodeURIComponent(accountId)}/smiles-statement/entries`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao carregar o extrato." }
    }
    const parsed = statementEntriesResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, entries: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
