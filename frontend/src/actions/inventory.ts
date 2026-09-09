"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"

const INVENTORY_PATH = "/dashboard/sales/inventory"
const ORIGINS_PATH = "/dashboard/point-batch-origins"

async function authHeaders() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${token}`,
  }
}

// Reads the API's `{success, message, data}` envelope, preferring the server's own message
// over a generic one: the backend already says why in Portuguese - "Já existe um período
// fechado que termina nesta data", "O custo destas emissões vem do estoque" - and replacing
// that with "erro ao gravar" throws away the only useful part.
async function send(url: string, init: RequestInit, fallback: string) {
  try {
    const res = await fetch(url, init)
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || fallback }
    }
    revalidatePath(INVENTORY_PATH)
    revalidatePath(ORIGINS_PATH)
    return { success: true as const, data: json?.data ?? null }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export interface PointBatchInput {
  accountId: string
  originId?: string | null
  acquiredOn: string
  points: number
  costCents: number
  fundingAgencyId: string
  profitSharePercent?: number | null
  opening?: boolean
  note?: string | null
}

// Records one acquisition of points. `opening` is the starting balance an account is put
// on the inventory with, and is the one batch that needs no origin - it was carried in,
// not bought from anybody.
export async function createPointBatchAction(input: PointBatchInput) {
  return send(
    `${env.BACKEND_URL}/api/v1/point-batches`,
    {
      method: "POST",
      headers: await authHeaders(),
      body: JSON.stringify({
        account_id: input.accountId,
        origin_id: input.originId ?? undefined,
        acquired_on: input.acquiredOn,
        points: input.points,
        cost_cents: input.costCents,
        funding_agency_id: input.fundingAgencyId,
        profit_share_percent: input.profitSharePercent ?? undefined,
        opening: input.opening ?? false,
        note: input.note ?? undefined,
      }),
    },
    "Erro ao lançar o lote.",
  )
}

// Corrects a batch. The account and the opening flag are not editable: moving a batch to
// another account would re-price two histories at once, so that is a delete and a new
// entry, where the intent is visible.
export async function updatePointBatchAction(
  batchId: string,
  input: Partial<Omit<PointBatchInput, "accountId" | "opening">>,
) {
  return send(
    `${env.BACKEND_URL}/api/v1/point-batches/${batchId}`,
    {
      method: "PUT",
      headers: await authHeaders(),
      body: JSON.stringify({
        origin_id: input.originId ?? undefined,
        acquired_on: input.acquiredOn ?? undefined,
        points: input.points ?? undefined,
        cost_cents: input.costCents ?? undefined,
        funding_agency_id: input.fundingAgencyId ?? undefined,
        profit_share_percent: input.profitSharePercent ?? undefined,
        note: input.note ?? undefined,
      }),
    },
    "Erro ao atualizar o lote.",
  )
}

// Archives a batch. The bookings it covered are re-queued against what remains on the next
// recompute, so this is safe on a batch entered by mistake - and it does not erase what it
// costed on the months already closed.
export async function deletePointBatchAction(batchId: string) {
  return send(
    `${env.BACKEND_URL}/api/v1/point-batches/${batchId}`,
    { method: "DELETE", headers: await authHeaders() },
    "Erro ao excluir o lote.",
  )
}

// Marks the batch settled between the agencies - the spreadsheet's "ACERTADO" column.
export async function settlePointBatchAction(batchId: string, settled: boolean) {
  return send(
    `${env.BACKEND_URL}/api/v1/point-batches/${batchId}/settlement`,
    {
      method: "PATCH",
      headers: await authHeaders(),
      body: JSON.stringify({ settled }),
    },
    "Erro ao registrar o acerto.",
  )
}

// Freezes a period. From here the costing inside it stops being recomputed, and a
// cancellation of anything in it reverses into the open period instead.
export async function closeInventoryPeriodAction(
  accountId: string,
  startsOn: string,
  endsOn: string,
) {
  return send(
    `${env.BACKEND_URL}/api/v1/inventory-periods`,
    {
      method: "POST",
      headers: await authHeaders(),
      body: JSON.stringify({ account_id: accountId, starts_on: startsOn, ends_on: endsOn }),
    },
    "Erro ao fechar o período.",
  )
}

// Undoes a closing. Only the account's most recent closed period comes off - the backend
// refuses the rest, because the periods are a chain and each one starts from what the last
// left behind. The settlement already recorded on the batches is not undone: closing and
// settling are separate acts.
export async function reopenInventoryPeriodAction(periodId: string) {
  return send(
    `${env.BACKEND_URL}/api/v1/inventory-periods/${periodId}`,
    { method: "DELETE", headers: await authHeaders() },
    "Erro ao reabrir o período.",
  )
}

// Renames an origin or retires it. Retiring is how an origin already used by a batch leaves
// the picker: deleting one is refused, so the batches keep their label.
export async function updatePointBatchOriginAction(
  originId: string,
  changes: { name?: string; active?: boolean },
) {
  return send(
    `${env.BACKEND_URL}/api/v1/point-batch-origins/${originId}`,
    {
      method: "PUT",
      headers: await authHeaders(),
      body: JSON.stringify(changes),
    },
    "Erro ao atualizar a origem.",
  )
}

export async function deletePointBatchOriginAction(originId: string) {
  return send(
    `${env.BACKEND_URL}/api/v1/point-batch-origins/${originId}`,
    { method: "DELETE", headers: await authHeaders() },
    "Erro ao excluir a origem.",
  )
}

// Adds an origin to the catalogue. Origins are a small, stable list - Livelo, Esfera, C6,
// Clube Azul - so this exists for the one that turns up rather than as a screen of its own.
export async function createPointBatchOriginAction(name: string) {
  return send(
    `${env.BACKEND_URL}/api/v1/point-batch-origins`,
    {
      method: "POST",
      headers: await authHeaders(),
      body: JSON.stringify({ name }),
    },
    "Erro ao criar a origem.",
  )
}
