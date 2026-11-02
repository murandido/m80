// The bookkeeping behind "Não salvo - Tentar de novo" on the board: which fields of a saved
// issuance a PATCH failed to write, by row key.
//
// Only the field names are kept, never the values. A retry sends what the row holds at the
// moment it is retried, so it can never write back an older value of a field the operator
// has typed over since - the same rule reconcile-rows.ts follows for server data.
export type UnsavedFields = Map<string, Set<string>>

// The waits before each automatic retry of a failed save. Short on purpose: what this
// covers is a blip (a deploy, a 429, a dropped connection). Anything longer is left to the
// "Tentar de novo" button, which stays on screen until the row saves.
export const AUTO_RETRY_DELAYS_MS = [2000, 5000, 15000]

// The wait before automatic retry number `attempt` (0-based), or null when they are spent.
export function autoRetryDelay(attempt: number): number | null {
  return AUTO_RETRY_DELAYS_MS[attempt] ?? null
}

// Called when a PATCH carrying `fields` is sent and again when it succeeds. On send: the
// newer value is on its way, and if it fails it is remembered again by the failure. On
// success: an earlier request for the same fields may have failed after this one was sent.
export function forgetFields(
  unsaved: UnsavedFields,
  rowKey: string,
  fields: string[],
): void {
  const pending = unsaved.get(rowKey)
  if (!pending) return
  for (const field of fields) pending.delete(field)
  if (pending.size === 0) unsaved.delete(rowKey)
}

export function rememberFields(
  unsaved: UnsavedFields,
  rowKey: string,
  fields: string[],
): void {
  if (fields.length === 0) return
  const pending = unsaved.get(rowKey) ?? new Set<string>()
  for (const field of fields) pending.add(field)
  unsaved.set(rowKey, pending)
}

// The patch a retry sends: the unsaved fields with the values the row shows now.
export function retryPatch<Row extends object>(
  row: Row,
  fields: Iterable<string>,
): Partial<Row> {
  const patch: Partial<Row> = {}
  for (const field of fields) {
    const key = field as keyof Row
    patch[key] = row[key]
  }
  return patch
}
