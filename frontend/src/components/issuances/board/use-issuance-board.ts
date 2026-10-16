"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { toast } from "sonner"
import { z } from "zod"
import type { GridReferenceData } from "@/components/issuances/grid-context"
import {
  IssuanceRow,
  draftIsReadyToCreate,
  issuanceToRow,
  makeDraftRow,
} from "@/components/issuances/types"
import { reconcileRows } from "@/components/issuances/reconcile-rows"
import { parseAccountEntry } from "@/components/issuances/account-entry"
import { useAutocompletePreference } from "@/hooks/use-autocomplete-preference"
import { useAirports } from "@/hooks/use-airports"
import { useSSE } from "@/hooks/use-sse"
import { defaultAgencyId } from "@/lib/default-agency"
import { normalizeSearch } from "@/lib/search"
import {
  attachAdministratorToAccountAction,
  createAccountForIssuanceAction,
} from "@/actions/account"
import {
  cancelIssuanceAction,
  createIssuanceAction,
  deleteIssuanceAction,
  deleteIssuanceAdjustmentAction,
  saveIssuanceAdjustmentAction,
  syncIssuanceWithTraveosAction,
  uncancelIssuanceAction,
  updateIssuanceAction,
} from "@/actions/issuance"
import { lookupIssuanceBookingAction } from "@/actions/issuance-lookup"
import {
  CancelIssuanceInput,
  Issuance,
  IssuanceAdjustment,
  IssuanceAdjustmentInput,
  getIssuancesSchema,
  updateIssuanceSchema,
} from "@/schemas/issuance"
import type { AccountOption } from "@/schemas/account"
import type { Agency } from "@/schemas/agency"
import type { Airline } from "@/schemas/airline"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"

// The state behind the issuances board: the rows, their autosave, the live refresh and the
// itinerary lookups. Ported from issuances-grid.tsx rather than extracted out of it: the
// board was built on the ca-mode branch, where every merge from main touched the grid. What
// is shared (types, reconcileRows, the actions) is imported, not copied.
//
// What the grid has and this does not: the cell selection machine, paste capture and the
// undo stack. A form has native focus, native paste and per-field undo.

export type ActionResult = { success: true } | { error: string }

// Whether anything can fetch this airline's itinerary at all. Mirrors the backend's
// resolveProviders: the airline's own integration when it is switched on, Traveos behind
// it when the airline is still supported there. autocomplete.ts only knows the Traveos
// half, which predates the direct integrations.
export function airlineAutocompletes(airline: Airline | undefined): boolean {
  if (!airline) return false
  if (airline.direct_booking_enabled) return true
  return airline.traveos_supported && airline.traveos_airline_id != null
}

// Today as the operator's calendar day - the browser's own zone, the one the screen is read in.
export function todayISO(): string {
  const now = new Date()
  return new Date(now.getTime() - now.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 10)
}

// A draft nobody touched: "Nova emissão" reopens it instead of piling up empty cards.
export function isUntouchedDraft(row: IssuanceRow): boolean {
  return (
    row.isDraft &&
    row.program_locator.trim() === "" &&
    row.loyalty_program_id === null &&
    row.account_id === null &&
    row.points === null &&
    row.value_of_the_thousand === null
  )
}

// Whether a draft holds anything the operator would lose - any field, not only the five a
// POST needs.
function draftHasTypedContent(row: IssuanceRow): boolean {
  return (
    row.program_locator.trim() !== "" ||
    [
      row.loyalty_program_id,
      row.airline_id,
      row.airline_locator,
      row.latam_purchase_number,
      row.flight_date,
      row.main_passenger_name,
      row.passenger_last_name,
      row.origin_iata_code,
      row.destination_iata_code,
      row.points,
      row.value_of_the_thousand,
      row.taxes,
      row.account_id,
      row.observation,
    ].some((value) => value !== null && value !== "")
  )
}

// Same trick as the grid's readCarriedDrafts: the page keys the board on its filters, so a
// filter change remounts it, and a half-typed draft lived only in the old instance's state.
// The old instance publishes a live view while mounted and a snapshot on unmount.
let readCarriedDrafts: () => IssuanceRow[] = () => []

// Where this browser keeps the drafts between reloads. Per browser on purpose: a draft is
// one operator's unfinished typing, not something another machine should be handed.
const DRAFTS_STORAGE_KEY = "issuances:drafts"

function readStoredDrafts(): IssuanceRow[] {
  try {
    const parsed: unknown = JSON.parse(
      window.localStorage.getItem(DRAFTS_STORAGE_KEY) ?? "[]",
    )
    if (!Array.isArray(parsed)) return []
    // Only what is recognisably a draft of ours: a row with an id is a saved issuance,
    // which the server - not this storage - is the source of.
    return parsed.filter(
      (row): row is IssuanceRow =>
        typeof row === "object" &&
        row !== null &&
        row.isDraft === true &&
        row.id === null &&
        typeof row.key === "string",
    )
  } catch {
    // Storage blocked or a value from some older shape: start without it.
    return []
  }
}

function writeStoredDrafts(drafts: IssuanceRow[]) {
  try {
    if (drafts.length === 0) {
      window.localStorage.removeItem(DRAFTS_STORAGE_KEY)
      return
    }
    window.localStorage.setItem(
      DRAFTS_STORAGE_KEY,
      JSON.stringify(
        drafts.map((row) => ({ ...row, saveStatus: "idle", hasError: false })),
      ),
    )
  } catch {
    // Without storage a draft still lives as long as the tab does.
  }
}

function mergeById<T extends { id: string }>(fromServer: T[], extra: T[]): T[] {
  if (extra.length === 0) return fromServer
  const byId = new Map(fromServer.map((item) => [item.id, item]))
  for (const item of extra) {
    if (!byId.has(item.id)) byId.set(item.id, item)
  }
  return [...byId.values()]
}

function absorb<T extends { id: string }>(
  known: T[],
  fromServer: T[],
  candidates: (T | null | undefined)[],
): T[] {
  const seen = new Set([...known, ...fromServer].map((item) => item.id))
  const added: T[] = []
  for (const candidate of candidates) {
    if (!candidate || seen.has(candidate.id)) continue
    seen.add(candidate.id)
    added.push(candidate)
  }
  return added.length > 0 ? [...known, ...added] : known
}

// What identifies the booking at the airline. A successful lookup is remembered against
// this, so filling the passenger's full name afterwards does not ask the airline again.
function bookingKey(row: IssuanceRow): string {
  return [
    row.airline_id,
    row.program_locator.trim().toUpperCase(),
    (row.airline_locator ?? "").trim().toUpperCase(),
    (row.latam_purchase_number ?? "").trim(),
  ].join("|")
}

// The surname the airline is asked with: the one typed for the lookup, else the main
// passenger's - the frontend twin of data.Issuance.BookingLastName.
export function bookingLastName(row: IssuanceRow): string {
  return (
    (row.passenger_last_name ?? "").trim() ||
    (row.main_passenger_name ?? "").trim()
  )
}

// Whether the row carries what this airline is asked with. autocomplete.ts has the grid's
// version, which only knows main_passenger_name.
export function hasLookupFields(row: IssuanceRow, airline: Airline): boolean {
  const locator = (row.airline_locator || row.program_locator || "").trim()
  if (!locator) return false
  if (airline.requires_origin && !(row.origin_iata_code ?? "").trim())
    return false
  if (airline.requires_last_name && !bookingLastName(row)) return false
  if (
    airline.requires_purchase_number &&
    !(row.latam_purchase_number ?? "").trim()
  )
    return false
  return true
}

// Everything a lookup is asked with. A failed attempt is remembered against this, so it is
// not repeated until the operator changes one of them - a typo fixed in the surname or the
// origin is exactly what should be tried again.
function attemptKey(row: IssuanceRow): string {
  return [
    bookingKey(row),
    (row.origin_iata_code ?? "").trim().toUpperCase(),
    normalizeSearch(bookingLastName(row)),
  ].join("|")
}

// The saved-row half of the autocomplete: the same sweep the grid runs, with the airline
// rule above instead of autocomplete.ts's Traveos-only one.
function isSyncCandidate(
  row: IssuanceRow,
  airlines: Airline[],
  attempted: Set<string>,
): boolean {
  if (row.isDraft || !row.id) return false
  if (
    row.status === "cancelled" ||
    row.traveos_synced_at ||
    row.traveos_syncing_at
  )
    return false
  if (attempted.has(row.key)) return false
  if (row.imported && row.traveos_attempted_at) return false
  const airline = airlines.find((candidate) => candidate.id === row.airline_id)
  if (!airline || !airlineAutocompletes(airline)) return false
  return hasLookupFields(row, airline)
}

interface UseIssuanceBoardOptions {
  initialIssuances: Issuance[]
  currentUserId: string | null
  filterQuery: string
  referenceData: GridReferenceData
}

export function useIssuanceBoard({
  initialIssuances,
  currentUserId,
  filterQuery,
  referenceData,
}: UseIssuanceBoardOptions) {
  const airports = useAirports()
  const [rows, setRows] = useState<IssuanceRow[]>(() => [
    ...initialIssuances.map(issuanceToRow),
    ...readCarriedDrafts(),
  ])
  const rowsRef = useRef(rows)
  useEffect(() => {
    rowsRef.current = rows
  }, [rows])

  const [extraAccounts, setExtraAccounts] = useState<AccountOption[]>([])
  const [extraAgencies, setExtraAgencies] = useState<Agency[]>([])
  const [extraPrograms, setExtraPrograms] = useState<LoyaltyProgram[]>([])
  const accounts = useMemo(
    () => mergeById(referenceData.accounts, extraAccounts),
    [referenceData.accounts, extraAccounts],
  )
  const agencies = useMemo(
    () => mergeById(referenceData.agencies, extraAgencies),
    [referenceData.agencies, extraAgencies],
  )
  const loyaltyPrograms = useMemo(
    () => mergeById(referenceData.loyaltyPrograms, extraPrograms),
    [referenceData.loyaltyPrograms, extraPrograms],
  )
  const accountsRef = useRef(accounts)
  useEffect(() => {
    accountsRef.current = accounts
  }, [accounts])

  const draftAgencyId = useMemo(
    () => defaultAgencyId(referenceData.agencies) ?? null,
    [referenceData.agencies],
  )

  // Rows whose account the program change wiped, until a new one is picked.
  const [accountClearedKeys, setAccountClearedKeys] = useState(
    new Set<string>(),
  )
  const [pendingAccountLabels, setPendingAccountLabels] = useState<
    Record<string, string>
  >({})
  const [syncingRowKeys, setSyncingRowKeys] = useState<Record<string, true>>({})
  const syncingRowKeysRef = useRef(syncingRowKeys)
  useEffect(() => {
    syncingRowKeysRef.current = syncingRowKeys
  }, [syncingRowKeys])
  const [lookingUpKeys, setLookingUpKeys] = useState<Record<string, true>>({})

  const [autocompleteEnabled, setAutocompleteEnabled] =
    useAutocompletePreference()

  const creatingKeysRef = useRef(new Set<string>())
  const queuedPatchesRef = useRef(new Map<string, Partial<IssuanceRow>>())

  // Publishes this board's unsaved drafts for the next instance - see readCarriedDrafts.
  useEffect(() => {
    const unsavedDrafts = () =>
      rowsRef.current.filter(
        (row) =>
          row.isDraft &&
          draftHasTypedContent(row) &&
          !creatingKeysRef.current.has(row.key),
      )
    readCarriedDrafts = unsavedDrafts
    return () => {
      const snapshot = unsavedDrafts()
      readCarriedDrafts = () => snapshot
    }
  }, [])

  // Drafts outlive a reload too: they are written to this browser's storage on every change
  // and put back when the board mounts. Read in an effect rather than in the initializer -
  // the server renders without storage, and the first client render has to match it.
  useEffect(() => {
    const stored = readStoredDrafts()
    if (stored.length === 0) return
    // eslint-disable-next-line react-hooks/set-state-in-effect -- drafts come from localStorage, which only exists after hydration
    setRows((prev) => {
      const known = new Set(prev.map((row) => row.key))
      const missing = stored.filter((row) => !known.has(row.key))
      return missing.length > 0 ? [...prev, ...missing] : prev
    })
  }, [])

  useEffect(() => {
    writeStoredDrafts(
      rows.filter(
        (row) =>
          row.isDraft &&
          draftHasTypedContent(row) &&
          // One whose create is in flight is left out: reloaded mid-POST, it would be
          // created a second time from storage.
          !creatingKeysRef.current.has(row.key),
      ),
    )
  }, [rows])

  // A draft survives a reload now; what does not is a save still travelling, whose
  // outcome would never be seen.
  useEffect(() => {
    function warnAboutUnsavedWork(event: BeforeUnloadEvent) {
      const saving =
        creatingKeysRef.current.size > 0 ||
        rowsRef.current.some((row) => row.saveStatus === "saving")
      if (!saving) return
      event.preventDefault()
      event.returnValue = ""
    }
    window.addEventListener("beforeunload", warnAboutUnsavedWork)
    return () =>
      window.removeEventListener("beforeunload", warnAboutUnsavedWork)
  }, [])

  const updateRow = useCallback((key: string, patch: Partial<IssuanceRow>) => {
    setRows((prev) =>
      prev.map((row) => (row.key === key ? { ...row, ...patch } : row)),
    )
  }, [])

  const flashSaveStatus = useCallback(
    (key: string, status: "success" | "error") => {
      updateRow(key, { saveStatus: status, hasError: status === "error" })
      setTimeout(() => updateRow(key, { saveStatus: "idle" }), 900)
    },
    [updateRow],
  )

  // See the grid's reconcileCreatedRow: the draft's key is kept so the open panel does not
  // lose its row, and edits typed while the POST was in flight win over the response.
  const reconcileCreatedRow = useCallback(
    (key: string, issuance: Issuance, pending?: Partial<IssuanceRow>) => {
      const reconcile = (prev: IssuanceRow[]) =>
        prev.map((row) =>
          row.key === key
            ? {
                ...issuanceToRow(issuance),
                ...pending,
                key,
                saveStatus: row.saveStatus,
              }
            : row,
        )
      // The ref follows at once rather than on the next effect: a field committed in
      // between would still see a draft whose create has finished, and POST it again.
      rowsRef.current = reconcile(rowsRef.current)
      setRows(reconcile)
    },
    [],
  )

  const persistRef =
    useRef<(row: IssuanceRow, patch: Partial<IssuanceRow>) => Promise<void>>(
      null,
    )

  const persist = useCallback(
    async (row: IssuanceRow, patch: Partial<IssuanceRow>) => {
      const merged = { ...row, ...patch }

      if (row.isDraft) {
        if (creatingKeysRef.current.has(row.key)) {
          queuedPatchesRef.current.set(row.key, {
            ...queuedPatchesRef.current.get(row.key),
            ...patch,
          })
          return
        }
        if (!draftIsReadyToCreate(merged)) return

        creatingKeysRef.current.add(row.key)
        updateRow(row.key, { saveStatus: "saving" })
        const result = await createIssuanceAction({
          program_locator: merged.program_locator,
          loyalty_program_id: merged.loyalty_program_id!,
          account_id: merged.account_id!,
          account_nickname_id: merged.account_nickname_id ?? undefined,
          points: merged.points!,
          value_of_the_thousand: merged.value_of_the_thousand!,
          airline_id: merged.airline_id ?? undefined,
          agency_id: merged.agency_id ?? undefined,
          issuer_id: merged.issuer_id ?? undefined,
          airline_locator: merged.airline_locator || undefined,
          latam_purchase_number: merged.latam_purchase_number || undefined,
          main_passenger_name: merged.main_passenger_name ?? undefined,
          passenger_last_name: merged.passenger_last_name || undefined,
          origin_iata_code: merged.origin_iata_code ?? undefined,
          destination_iata_code: merged.destination_iata_code ?? undefined,
          taxes: merged.taxes ?? undefined,
          observation: merged.observation ?? undefined,
          flight_date: merged.flight_date ?? undefined,
          issued_on: merged.issued_on,
        }).finally(() => creatingKeysRef.current.delete(row.key))

        const queued = queuedPatchesRef.current.get(row.key)
        queuedPatchesRef.current.delete(row.key)

        if ("error" in result) {
          toast.error(result.error)
          flashSaveStatus(row.key, "error")
          return
        }

        reconcileCreatedRow(row.key, result.issuance, queued)
        flashSaveStatus(row.key, "success")

        if (queued) {
          await persistRef.current?.(
            { ...merged, key: row.key, id: result.issuance.id, isDraft: false },
            queued,
          )
        }
        return
      }

      updateRow(row.key, { saveStatus: "saving" })
      const result = await updateIssuanceAction(
        row.id!,
        patch as z.infer<typeof updateIssuanceSchema.request>,
      )
      if ("error" in result) {
        toast.error(result.error)
        flashSaveStatus(row.key, "error")
        return
      }
      if (result.issuance) {
        updateRow(row.key, {
          status: result.issuance.status,
          redemption_fee_saved_cents:
            result.issuance.redemption_fee_saved_cents ?? null,
          redemption_fee_manual: result.issuance.redemption_fee_manual,
        })
      }
      flashSaveStatus(row.key, "success")
    },
    [updateRow, flashSaveStatus, reconcileCreatedRow],
  )
  // eslint-disable-next-line react-hooks/refs -- latest-ref read by callbacks memoized without persist in their deps
  persistRef.current = persist

  // The single entry point for an edit: optimistic, then persisted. Addressed by key, since
  // the panel outlives index shifts from a live refresh.
  const applyPatch = useCallback(
    (rowKey: string, patch: Partial<IssuanceRow>) => {
      const row = rowsRef.current.find((candidate) => candidate.key === rowKey)
      if (!row) return
      // Drops fields that would not change anything, so tabbing through a form does not
      // fire a PATCH per field.
      const changed = Object.fromEntries(
        Object.entries(patch).filter(
          ([key, value]) => row[key as keyof IssuanceRow] !== value,
        ),
      ) as Partial<IssuanceRow>
      if (Object.keys(changed).length === 0) return

      if ("account_id" in changed) {
        setAccountClearedKeys((prev) => {
          const next = new Set(prev)
          if (changed.account_id === null) next.add(row.key)
          else next.delete(row.key)
          return next
        })
      }
      // Kept in step synchronously as well as through the effect: a field's blur and the
      // next field's commit can land in the same tick, and the second must see the first.
      const next = { ...row, ...changed }
      rowsRef.current = rowsRef.current.map((candidate) =>
        candidate.key === rowKey ? next : candidate,
      )
      updateRow(row.key, changed)
      void persist(row, changed)
    },
    [updateRow, persist],
  )

  // Live updates, same as the grid: every issuance mutation refetches the filtered list.
  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const adoptReferences = useCallback(
    (fresh: Issuance[]) => {
      setExtraAccounts((prev) =>
        absorb(
          prev,
          referenceData.accounts,
          fresh.map((issuance) => issuance.account),
        ),
      )
      setExtraAgencies((prev) =>
        absorb(
          prev,
          referenceData.agencies,
          fresh.map((issuance) => issuance.agency),
        ),
      )
      setExtraPrograms((prev) =>
        absorb(
          prev,
          referenceData.loyaltyPrograms,
          fresh.map((issuance) => issuance.loyalty_program),
        ),
      )
    },
    [
      referenceData.accounts,
      referenceData.agencies,
      referenceData.loyaltyPrograms,
    ],
  )

  // The row open in the panel is the one being typed into; reconcileRows keeps its local
  // values over the server's, the way the grid protects the cell being edited.
  const openKeyRef = useRef<string | null>(null)

  const refreshFromServer = useCallback(async () => {
    let fresh: Issuance[]
    try {
      const res = await fetch(`/api/issuances?${filterQuery}`, {
        cache: "no-store",
      })
      const parsed = getIssuancesSchema.response.safeParse(await res.json())
      if (!res.ok || !parsed.success) return
      fresh = parsed.data.data ?? []
    } catch {
      return
    }
    adoptReferences(fresh)
    setRows(reconcileRows(rowsRef.current, fresh, openKeyRef.current))
  }, [filterQuery, adoptReferences])

  const scheduleRefresh = useCallback(() => {
    if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current)
    refreshTimeoutRef.current = setTimeout(() => void refreshFromServer(), 300)
  }, [refreshFromServer])

  useEffect(
    () => () => {
      if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current)
    },
    [],
  )

  useSSE(
    "/api/events",
    {
      "issuance.created": scheduleRefresh,
      "issuance.updated": scheduleRefresh,
      "issuance.cancelled": scheduleRefresh,
      "issuance.deleted": scheduleRefresh,
    },
    { onReconnect: scheduleRefresh },
  )

  // Opens a new draft, reusing an untouched one so "Nova emissão" twice does not leave an
  // empty card behind. Returns the key the panel should open.
  const addDraft = useCallback((): string => {
    const untouched = rowsRef.current.find(isUntouchedDraft)
    if (untouched) return untouched.key
    const draft = makeDraftRow(currentUserId, draftAgencyId, todayISO())
    rowsRef.current = [...rowsRef.current, draft]
    setRows((prev) => [...prev, draft])
    return draft.key
  }, [currentUserId, draftAgencyId])

  const removeRow = useCallback((key: string) => {
    setRows((prev) => prev.filter((row) => row.key !== key))
    setAccountClearedKeys((prev) => {
      if (!prev.has(key)) return prev
      const next = new Set(prev)
      next.delete(key)
      return next
    })
  }, [])

  const deleteRow = useCallback(
    async (key: string): Promise<ActionResult> => {
      const row = rowsRef.current.find((candidate) => candidate.key === key)
      if (!row) return { error: "Emissão não encontrada." }
      if (row.isDraft) {
        removeRow(key)
        return { success: true }
      }
      const result = await deleteIssuanceAction(row.id!)
      if ("error" in result)
        return { error: result.error ?? "Erro ao deletar emissão." }
      removeRow(key)
      return { success: true }
    },
    [removeRow],
  )

  const cancelRow = useCallback(
    async (key: string, data: CancelIssuanceInput): Promise<ActionResult> => {
      const row = rowsRef.current.find((candidate) => candidate.key === key)
      if (!row || row.isDraft || !row.id)
        return { error: "Emissão ainda não foi salva." }
      const result = await cancelIssuanceAction(row.id, data)
      if ("error" in result)
        return { error: result.error ?? "Erro ao cancelar emissão." }
      updateRow(row.key, {
        status: "cancelled",
        cancelled_at: result.issuance?.cancelled_at ?? null,
        cancelled_by_id: result.issuance?.cancelled_by_id ?? null,
        refunded_points: result.issuance?.refunded_points ?? null,
        refunded_taxes: result.issuance?.refunded_taxes ?? null,
        cancellation_fee: result.issuance?.cancellation_fee ?? null,
        cancellation_only: result.issuance?.cancellation_only ?? false,
      })
      return { success: true }
    },
    [updateRow],
  )

  const uncancelRow = useCallback(
    async (key: string): Promise<ActionResult> => {
      const row = rowsRef.current.find((candidate) => candidate.key === key)
      if (!row || row.isDraft || !row.id)
        return { error: "Emissão ainda não foi salva." }
      const result = await uncancelIssuanceAction(row.id)
      if ("error" in result)
        return { error: result.error ?? "Erro ao reverter cancelamento." }
      updateRow(row.key, {
        status: result.issuance?.status ?? "incomplete",
        cancelled_at: null,
        cancelled_by_id: null,
        refunded_points: null,
        refunded_taxes: null,
        cancellation_fee: null,
        cancellation_only: false,
      })
      return { success: true }
    },
    [updateRow],
  )

  // An alteration is written through its own endpoint and folded into the row from the
  // response, not from a refresh: the open panel's row is protected from the refresh (see
  // reconcileRows), so waiting for the SSE round trip would leave it showing the old list.
  const saveAdjustment = useCallback(
    async (
      key: string,
      adjustmentId: string | null,
      data: IssuanceAdjustmentInput,
    ): Promise<ActionResult> => {
      const row = rowsRef.current.find((candidate) => candidate.key === key)
      if (!row || row.isDraft || !row.id)
        return { error: "Emissão ainda não foi salva." }
      const result = await saveIssuanceAdjustmentAction(row.id, adjustmentId, data)
      if ("error" in result)
        return { error: result.error ?? "Erro ao salvar a alteração." }
      const saved = result.adjustment
      setRows((prev) =>
        prev.map((candidate) => {
          if (candidate.key !== key) return candidate
          const others = candidate.adjustments.filter(
            (adjustment) => adjustment.id !== saved.id,
          )
          return {
            ...candidate,
            adjustments: [...others, saved].sort(byOccurrence),
          }
        }),
      )
      return { success: true }
    },
    [],
  )

  const deleteAdjustment = useCallback(
    async (key: string, adjustmentId: string): Promise<ActionResult> => {
      const row = rowsRef.current.find((candidate) => candidate.key === key)
      if (!row || row.isDraft || !row.id)
        return { error: "Emissão ainda não foi salva." }
      const result = await deleteIssuanceAdjustmentAction(row.id, adjustmentId)
      if ("error" in result)
        return { error: result.error ?? "Erro ao excluir a alteração." }
      setRows((prev) =>
        prev.map((candidate) =>
          candidate.key === key
            ? {
                ...candidate,
                adjustments: candidate.adjustments.filter(
                  (adjustment) => adjustment.id !== adjustmentId,
                ),
              }
            : candidate,
        ),
      )
      return { success: true }
    },
    [],
  )

  // Pulls and writes the itinerary of a saved issuance - the button, and the sweep below.
  const syncRow = useCallback(
    async (key: string): Promise<ActionResult> => {
      const row = rowsRef.current.find((candidate) => candidate.key === key)
      if (!row || row.isDraft || !row.id)
        return { error: "Emissão ainda não foi salva." }
      if (syncingRowKeysRef.current[row.key] || row.traveos_syncing_at) {
        return { error: "Os dados dessa emissão já estão sendo puxados." }
      }

      setSyncingRowKeys((prev) => ({ ...prev, [row.key]: true }))
      // The taxes as they were when the pull started, to tell whether the operator has typed
      // over them while it ran.
      const taxesBefore = row.taxes
      try {
        const result = await syncIssuanceWithTraveosAction(row.id)
        if ("error" in result)
          return { error: result.error ?? "Erro ao puxar os dados da cia." }
        const issuance = result.issuance
        // On an exempt account (Azul's Diamante Unique) the sync works out the redemption fee
        // and folds it into the taxes (issuance.RecalculateRedemptionFee). The grid picked that
        // up from the refresh that followed; the board cannot, because the open panel's row is
        // protected from refreshes - so the response is applied here. The taxes only when the
        // operator has not typed over them meanwhile: that edit is the newer one.
        const current = rowsRef.current.find((candidate) => candidate.key === key)
        const feeFields: Partial<IssuanceRow> = issuance
          ? {
              redemption_fee_saved_cents: issuance.redemption_fee_saved_cents ?? null,
              redemption_fee_manual: issuance.redemption_fee_manual,
              ...(current?.taxes === taxesBefore ? { taxes: issuance.taxes } : {}),
            }
          : {}
        updateRow(row.key, {
          ...feeFields,
          status: issuance?.status ?? "complete",
          traveos_synced_at:
            issuance?.traveos_synced_at ?? new Date().toISOString(),
          traveos_syncing_at: null,
          main_passenger_name:
            issuance?.main_passenger_name ?? row.main_passenger_name,
          origin_iata_code: issuance?.origin_iata_code ?? row.origin_iata_code,
          destination_iata_code:
            issuance?.destination_iata_code ?? row.destination_iata_code,
          flight_date:
            issuance?.flights?.[0]?.departure_at.slice(0, 10) ??
            row.flight_date,
          latam_purchase_number:
            issuance?.latam_purchase_number ?? row.latam_purchase_number,
        })
        return { success: true }
      } finally {
        setSyncingRowKeys((prev) => {
          const next = { ...prev }
          delete next[row.key]
          return next
        })
      }
    },
    [updateRow],
  )

  // The saved half of the autocomplete: once a row is saved, its itinerary is written by the
  // real sync, one row at a time - the same sweep the grid runs over everything on screen.
  const syncAttemptedRef = useRef(new Set<string>())
  // Lookups that failed, by everything they were asked with (see attemptKey). The sync after
  // the save would ask the airline the same question and get the same answer.
  const failedLookupsRef = useRef(new Set<string>())
  const [sweepRunning, setSweepRunning] = useState(false)
  useEffect(() => {
    if (!autocompleteEnabled || sweepRunning) return
    const candidate = rowsRef.current.find(
      (row) =>
        // Never beside a lookup still running for the same booking: with the vendor as
        // the provider, two at once can each import it - two real tickets for one.
        !lookingUpKeys[row.key] &&
        !failedLookupsRef.current.has(attemptKey(row)) &&
        isSyncCandidate(row, referenceData.airlines, syncAttemptedRef.current),
    )
    if (!candidate) return
    syncAttemptedRef.current.add(candidate.key)
    setSweepRunning(true)
    void syncRow(candidate.key)
      .then((result) => {
        if ("error" in result) toast.error(result.error)
      })
      .finally(() => setSweepRunning(false))
  }, [
    rows,
    autocompleteEnabled,
    sweepRunning,
    lookingUpKeys,
    referenceData.airlines,
    syncRow,
  ])

  // The draft half: the itinerary preview, before the values that saving needs. Fires once
  // per set of identifying fields, and only for drafts - a saved row has the real sync.
  const lookupAttemptedRef = useRef(new Set<string>())
  const lookedUpRef = useRef(new Map<string, string>())
  useEffect(() => {
    if (!autocompleteEnabled) return
    for (const row of rows) {
      if (!row.isDraft || lookingUpKeys[row.key]) continue
      const airline = referenceData.airlines.find(
        (candidate) => candidate.id === row.airline_id,
      )
      if (!airlineAutocompletes(airline) || !hasLookupFields(row, airline!))
        continue
      if (lookedUpRef.current.get(row.key) === bookingKey(row)) continue
      const attempt = attemptKey(row)
      if (lookupAttemptedRef.current.has(attempt)) continue
      lookupAttemptedRef.current.add(attempt)

      const rowKey = row.key
      const sentBookingKey = bookingKey(row)
      setLookingUpKeys((prev) => ({ ...prev, [rowKey]: true }))
      void lookupIssuanceBookingAction({
        airline_id: row.airline_id!,
        program_locator: row.program_locator.trim(),
        airline_locator: row.airline_locator?.trim() || undefined,
        origin_iata_code: row.origin_iata_code?.trim() || undefined,
        main_passenger_name: row.main_passenger_name?.trim() || undefined,
        passenger_last_name: row.passenger_last_name?.trim() || undefined,
        latam_purchase_number: row.latam_purchase_number?.trim() || undefined,
      })
        .then((result) => {
          if ("error" in result) {
            failedLookupsRef.current.add(attempt)
            toast.error(result.error)
            return
          }
          const current = rowsRef.current.find(
            (candidate) => candidate.key === rowKey,
          )
          // The operator changed the booking while it was being looked up: this answer is
          // about a booking that is no longer on the form.
          if (!current || bookingKey(current) !== sentBookingKey) return
          lookedUpRef.current.set(rowKey, sentBookingKey)
          const { preview } = result
          const patch: Partial<IssuanceRow> = {}
          if (preview.origin_iata_code)
            patch.origin_iata_code = preview.origin_iata_code
          if (preview.destination_iata_code)
            patch.destination_iata_code = preview.destination_iata_code
          if (preview.flight_date) patch.flight_date = preview.flight_date
          if (preview.main_passenger_name)
            patch.main_passenger_name = preview.main_passenger_name
          if (preview.latam_purchase_number)
            patch.latam_purchase_number = preview.latam_purchase_number
          applyPatch(rowKey, patch)
          toast.success("Reserva encontrada, dados do voo preenchidos.")
        })
        .finally(() =>
          setLookingUpKeys((prev) => {
            const next = { ...prev }
            delete next[rowKey]
            return next
          }),
        )
    }
  }, [
    rows,
    autocompleteEnabled,
    lookingUpKeys,
    referenceData.airlines,
    applyPatch,
  ])

  // "conta" or "conta - administrador" typed into the account field: reuse the alias when it
  // exists, create the trio when it does not. Same flow as the grid's.
  const resolveAccountEntry = useCallback(
    async (rowKey: string, typed: string) => {
      const row = rowsRef.current.find((candidate) => candidate.key === rowKey)
      if (!row?.loyalty_program_id) return
      const programId = row.loyalty_program_id
      const { nickname, administratorName } = parseAccountEntry(typed)

      const wanted = normalizeSearch(nickname)
      let existing: { accountId: string; nicknameId: string } | null = null
      for (const account of accountsRef.current) {
        if (account.deleted_at || account.loyalty_program_id !== programId)
          continue
        const match = account.nicknames.find(
          (entry) => normalizeSearch(entry.nickname) === wanted,
        )
        if (match) {
          existing = { accountId: account.id, nicknameId: match.id }
          break
        }
      }

      setPendingAccountLabels((prev) => ({ ...prev, [rowKey]: typed }))
      const clearPending = () =>
        setPendingAccountLabels((prev) => {
          const { [rowKey]: _removed, ...rest } = prev
          return rest
        })

      if (existing) {
        if (administratorName) {
          const linked = await attachAdministratorToAccountAction(
            existing.accountId,
            administratorName,
          )
          if ("error" in linked) {
            toast.error(linked.error)
          } else if ("conflict" in linked) {
            const current = referenceData.people.find(
              (person) => person.id === linked.currentAdministratorId,
            )
            toast.warning(
              `A conta "${nickname}" já tem ${current?.name ?? "outra pessoa"} como administrador. ` +
                `O vínculo não foi alterado.`,
            )
          }
        }
        clearPending()
        applyPatch(rowKey, {
          account_id: existing.accountId,
          account_nickname_id: existing.nicknameId,
        })
        return
      }

      const result = await createAccountForIssuanceAction(
        nickname,
        programId,
        administratorName,
      )
      clearPending()
      if ("error" in result) {
        toast.error(result.error)
        flashSaveStatus(rowKey, "error")
        return
      }
      setExtraAccounts((prev) => [...prev, result.account])
      applyPatch(rowKey, {
        account_id: result.account.id,
        account_nickname_id: result.nicknameId,
      })
    },
    [applyPatch, flashSaveStatus, referenceData.people],
  )

  return {
    rows,
    airports,
    accounts,
    agencies,
    loyaltyPrograms,
    airlines: referenceData.airlines,
    users: referenceData.users,
    people: referenceData.people,
    openKeyRef,
    accountClearedKeys,
    pendingAccountLabels,
    syncingRowKeys,
    lookingUpKeys,
    autocompleteEnabled,
    setAutocompleteEnabled,
    addDraft,
    applyPatch,
    resolveAccountEntry,
    deleteRow,
    cancelRow,
    uncancelRow,
    saveAdjustment,
    deleteAdjustment,
    syncRow,
  }
}

// The order the server lists alterations in: by day, then by when they were typed.
function byOccurrence(a: IssuanceAdjustment, b: IssuanceAdjustment): number {
  if (a.occurred_on !== b.occurred_on) return a.occurred_on < b.occurred_on ? -1 : 1
  return a.created_at < b.created_at ? -1 : a.created_at > b.created_at ? 1 : 0
}

export type IssuanceBoardState = ReturnType<typeof useIssuanceBoard>
