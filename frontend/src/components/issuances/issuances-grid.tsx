"use client"

import { type ReactNode, useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react"
import { Plus, Keyboard, FileSpreadsheet, AlertTriangle, CalendarDays, ChevronDown } from "lucide-react"
import { toast } from "sonner"
import { cn } from "@/lib/utils"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { DateInput } from "@/components/ui/date-input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { GridContextValue, GridProvider, GridReferenceData } from "@/components/issuances/grid-context"
import { GridCell } from "@/components/issuances/grid-cell"
import { RowActions } from "@/components/issuances/row-actions"
import { AutosaveDot } from "@/components/issuances/autosave-dot"
import { interactionReducer, initialInteractionState } from "@/components/issuances/interaction-reducer"
import {
  IssuanceRow,
  issuanceToRow,
  makeDraftRow,
  draftIsReadyToCreate,
  isRowSyncing,
} from "@/components/issuances/types"
import { buildCalculo, buildEmissaoResumo } from "@/components/issuances/emissao-message"
import { copyEmissaoText } from "@/components/issuances/copy-emissao-menu"
import { reconcileRows } from "@/components/issuances/reconcile-rows"
import {
  autocompleteHintFor,
  isAutocompleteCandidate,
} from "@/components/issuances/autocomplete"
import { useAutocompletePreference } from "@/hooks/use-autocomplete-preference"
import { useAirports } from "@/hooks/use-airports"
import { clearPatchForColumn } from "@/components/issuances/column-fields"
import { useSSE } from "@/hooks/use-sse"
import {
  ACCOUNT_COLUMN_INDEX,
  CALCULATED_COLUMN_INDEX,
  COLUMN_COUNT,
  COLUMN_COUNT_WITH_ISSUED_ON,
  LOCATOR_COLUMN_INDEX,
  OBSERVATION_COLUMN_INDEX,
  REDEMPTION_FEE_COLUMN_INDEX,
  TAXES_COLUMN_INDEX,
  clampMove,
  resolveNextCell,
  Direction,
} from "@/lib/grid-navigation"
import { Issuance, getIssuancesSchema, updateIssuanceSchema } from "@/schemas/issuance"
import { AccountOption } from "@/schemas/account"
import { Agency } from "@/schemas/agency"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import {
  createIssuanceAction,
  updateIssuanceAction,
  deleteIssuanceAction,
  cancelIssuanceAction,
  uncancelIssuanceAction,
  syncIssuanceWithTraveosAction,
} from "@/actions/issuance"
import { CancelIssuanceInput } from "@/schemas/issuance"
import { RedemptionFeeSummary } from "@/components/issuances/redemption-fee-summary"
import { acceptsRedemptionFee } from "@/components/issuances/redemption-fee"
import {
  attachAdministratorToAccountAction,
  createAccountForIssuanceAction,
} from "@/actions/account"
import { parseAccountEntry } from "@/components/issuances/account-entry"
import { normalizeSearch } from "@/lib/search"
import { z } from "zod"
import { defaultAgencyId } from "@/lib/default-agency"

const COLUMN_LABELS = [
  "Cia aérea",
  "Localizador",
  "Data do voo",
  "Passageiro principal",
  "Origem",
  "Destino",
  "Pontos",
  "Valor milheiro",
  "Taxas",
  "Valor total",
  "Conta",
  "Emissor",
  "Agência",
  "Status",
  // Last, not next to Agência: inserting it in the middle would renumber every
  // column index in grid-cell.tsx, column-fields.ts and grid-navigation.ts.
  "Observação",
  // Appended for the same reason, even though it reads as money and would sit more
  // naturally next to Taxas.
  "Taxa de resgate",
  // The day the issuance happened - normally today, and corrected when a booking is
  // recorded after the fact. Last of all, which is also what lets it be hidden without
  // moving any other column: see COLUMN_COUNT_WITH_ISSUED_ON.
  "Data da emissão",
]

// Overlays the records this session learned about on top of the server-rendered list. The
// server wins on collision: its copy came from a full fetch, the overlay from whatever a
// single issuance happened to carry.
function mergeById<T extends { id: string }>(fromServer: T[], extra: T[]): T[] {
  if (extra.length === 0) return fromServer

  const byId = new Map(fromServer.map((item) => [item.id, item]))
  for (const item of extra) {
    if (!byId.has(item.id)) byId.set(item.id, item)
  }
  return [...byId.values()]
}

// Picks out the candidates neither list knows yet. Returns `known` itself when there are
// none, so React bails out of the update instead of invalidating the grid context and
// re-rendering every cell on a routine refetch.
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

interface IssuancesGridProps extends GridReferenceData {
  initialIssuances: Issuance[]
  // Defaults a newly created draft row's "Emissor" - still freely editable.
  currentUserId: string | null
  // Rendered on the left of the sticky toolbar, sharing the row with the
  // Atalhos/Nova emissão actions. Supplied by the page (the filter bar).
  filters?: ReactNode
  // Current filter query string (e.g. "date_from=...&issuer_id=..."). Forwarded to the
  // export endpoint so the downloaded .xlsx matches the filtered list on screen, and
  // reused for the live refetch so an SSE-driven refresh applies the same filters.
  filterQuery: string
}

// One undoable step: the patch that restores a row to its prior state, keyed by the
// row's stable client id (not its index, which shifts as rows are added/removed).
interface HistoryEntry {
  key: string
  patch: Partial<IssuanceRow>
}

// Caps each history stack so a long editing session can't grow them without bound.
const HISTORY_LIMIT = 100

// Where this browser remembers whether the "Data da emissão" column is on screen.
const ISSUED_ON_COLUMN_STORAGE_KEY = "m80.issuances.showIssuedOn"

// Today as the operator's calendar day. The browser's own zone, which is the one they are
// reading the screen in - the server resolves the stored timestamp the same way.
function todayISO(): string {
  const now = new Date()
  return new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10)
}

// Shown in the "Atalhos" popover so the keyboard-first workflow is discoverable.
const SHORTCUTS: { keys: string; label: string }[] = [
  { keys: "Tab / Shift+Tab", label: "Próxima / célula anterior" },
  { keys: "Enter / Shift+Enter", label: "Célula abaixo / acima" },
  { keys: "Setas", label: "Mover a seleção" },
  { keys: "F2 ou digitar", label: "Editar a célula" },
  { keys: "Esc", label: "Cancelar a edição" },
  { keys: "Home / End", label: "Início / fim da linha" },
  { keys: "Ctrl+Home / Ctrl+End", label: "Início / fim da grade" },
  { keys: "Ctrl + setas", label: "Saltar até a borda" },
  { keys: "Ctrl+Enter", label: "Nova emissão" },
  { keys: "Delete", label: "Limpar a célula" },
  { keys: "Ctrl+C / Ctrl+V", label: "Copiar / colar" },
  { keys: "Ctrl+Alt+C", label: "Copiar a emissão como mensagem" },
  { keys: "Ctrl+Alt+X", label: "Copiar só o cálculo" },
  { keys: "Ctrl+Z / Ctrl+Shift+Z", label: "Desfazer / refazer" },
  { keys: "Ctrl+Shift+⌫", label: "Excluir a linha" },
  { keys: "/", label: 'Separar "programa/cia" (Cia aérea, Localizador)' },
]

// A cancelled issuance is terminal with one exception: the boarding taxes are still
// corrected after the fact (the airline refunds a different amount than was charged), so
// that one column stays writable while the rest of the row does not. Module scope, not a
// closure over render state: it reads only its arguments, and onCellClick - a []-deps
// callback - needs to call it without capturing a stale render.
function isLockedByCancellation(c: number, row: IssuanceRow | undefined): boolean {
  return row?.status === "cancelled" && c !== TAXES_COLUMN_INDEX
}

// A draft row nobody has touched yet - clicking "Nova emissão" repeatedly
// should focus this one instead of piling up empty rows.
function isUntouchedDraft(row: IssuanceRow): boolean {
  return (
    row.isDraft &&
    row.program_locator.trim() === "" &&
    row.loyalty_program_id === null &&
    row.account_id === null &&
    row.points === null &&
    row.value_of_the_thousand === null
  )
}

// Whether the operator typed anything into a draft - any cell, not only the five a POST
// needs. isUntouchedDraft answers a narrower question ("can this row be reused for Nova
// emissão?"), and a draft holding only a passenger and a route counts as untouched there,
// yet losing it still throws away work.
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

// Where a freshly mounted grid finds the unsaved drafts of the one before it. The page
// keys the grid on its filters, so every filter change remounts it - and a half-typed
// issuance (not yet the five fields a POST needs) lived only in that instance's state and
// vanished without a word.
//
// A getter rather than a list, because of the order React does things in: on a key
// change it renders the new instance - running its useState initializer - before it runs
// the old instance's cleanup. So the old instance publishes a live view of its rows while
// mounted, which the new one reads during that render, and freezes it into a snapshot on
// unmount for a grid mounted later (leaving the screen and coming back).
let readCarriedDrafts: () => IssuanceRow[] = () => []

export function IssuancesGrid({
  initialIssuances,
  currentUserId,
  filters,
  filterQuery,
  ...referenceData
}: IssuancesGridProps) {
  // Fetched from the browser rather than handed down with the page: 1.3 MB of reference
  // data does not belong in the payload of every navigation. Empty for the first moment
  // after mount, which the airport cell handles - it commits typed IATA codes regardless.
  const airports = useAirports()
  // Start from exactly what the server returned - no seeded draft row. An empty
  // result is a legitimate filtered outcome and gets an explicit empty-state below
  // (a lone editable draft row here reads as a stray/phantom line). "Nova emissão"
  // (handleAddRow / Ctrl+Enter) creates the first row on demand.
  const [rows, setRows] = useState<IssuanceRow[]>(() => [
    ...initialIssuances.map(issuanceToRow),
    // Drafts are always the tail, same as reconcileRows keeps them.
    ...readCarriedDrafts(),
  ])
  const [interaction, dispatch] = useReducer(interactionReducer, initialInteractionState)
  // The day new rows are recorded on. Normally today; set to another day to enter a
  // batch of bookings that happened then - the ones a statement reconciliation turned up.
  const [recordingOn, setRecordingOn] = useState(todayISO)
  // Whether the "Data da emissão" column is on screen. A preference, not state the server
  // knows about, so it lives in this browser. Read in an effect rather than in the
  // initializer because localStorage does not exist while the page is server-rendered.
  const [showIssuedOn, setShowIssuedOn] = useState(false)

  useEffect(() => {
    try {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- localStorage only exists after hydration
      setShowIssuedOn(window.localStorage.getItem(ISSUED_ON_COLUMN_STORAGE_KEY) === "1")
    } catch {
      // A browser with site data blocked still gets a working grid, just not a
      // remembered preference.
    }
  }, [])

  // Backdating deliberately is the one moment the column must not be hidden: the dates
  // being written are no longer the obvious ones.
  const backdating = recordingOn !== todayISO()
  const issuedOnVisible = showIssuedOn || backdating
  const columnCount = issuedOnVisible ? COLUMN_COUNT_WITH_ISSUED_ON : COLUMN_COUNT

  // The two callbacks below are memoized with dependency lists that must not include a
  // value changing this often, so they read the current one from a ref instead.
  const columnCountRef = useRef(columnCount)
  // eslint-disable-next-line react-hooks/refs -- latest-ref read by callbacks memoized without this value in their deps
  columnCountRef.current = columnCount
  const recordingOnRef = useRef(recordingOn)
  // eslint-disable-next-line react-hooks/refs -- latest-ref, same as columnCountRef above
  recordingOnRef.current = recordingOn

  function toggleIssuedOnColumn() {
    const next = !showIssuedOn
    setShowIssuedOn(next)
    try {
      window.localStorage.setItem(ISSUED_ON_COLUMN_STORAGE_KEY, next ? "1" : "0")
    } catch {
      // Same as above: the preference simply does not persist.
    }
    // The selection cannot be left on a column that is no longer rendered.
    if (!next && !backdating && interaction.sel.c >= COLUMN_COUNT) {
      dispatch({ type: "SELECT_ONLY", r: interaction.sel.r, c: COLUMN_COUNT - 1 })
    }
  }
  const activeCellSaveRef = useRef<(() => void) | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  // A hidden but focusable textarea that holds focus while a cell is selected
  // (not editing). It exists so Ctrl+V fires a native `paste` event we can read
  // synchronously from clipboardData - a focused non-editable <div> never
  // receives paste events, and navigator.clipboard.readText() prompts for a
  // clipboard-read permission on first use.
  const pasteCatcherRef = useRef<HTMLTextAreaElement>(null)
  // The agency a new draft row defaults to - the user's default one. The backend applies
  // the same fallback when agency_id is omitted, so this just surfaces it in the draft.
  const draftAgencyId = useMemo(
    () => defaultAgencyId(referenceData.agencies) ?? null,
    [referenceData.agencies],
  )
  // Rows whose POST is currently in flight. `row.isDraft` alone can't gate the create:
  // it comes from the current render, so a cell edited before the POST resolves would
  // still see isDraft === true and fire a second create for the same row.
  const creatingKeysRef = useRef(new Set<string>())
  // Edits made while a row's POST is in flight, replayed as a PATCH once the server
  // hands back the real id.
  const queuedPatchesRef = useRef(new Map<string, Partial<IssuanceRow>>())
  // Rows whose account the program change wiped, until a new one is picked. Session-only
  // by nature: the old account_id survives in the database (the column is NOT NULL), so a
  // reload brings it back and the highlight legitimately disappears.
  const [accountClearedKeys, setAccountClearedKeys] = useState(new Set<string>())
  // Accounts and agencies this session learned about after the page rendered: created
  // inline from the account cell, or arrived embedded in a refetched issuance (see
  // adoptReferences). Kept alongside the props rather than replacing them, so a later
  // refresh of the server-fetched lists doesn't get shadowed by a stale copy - the merges
  // below dedupe when the same record arrives from both.
  const [extraAccounts, setExtraAccounts] = useState<AccountOption[]>([])
  const [extraAgencies, setExtraAgencies] = useState<Agency[]>([])
  const [extraPrograms, setExtraPrograms] = useState<LoyaltyProgram[]>([])
  const [pendingAccountLabels, setPendingAccountLabels] = useState<Record<string, string>>({})
  // Keyed by row key rather than index: a Traveos sync can run for minutes, and the
  // row can move as the list reorders or reconciles while it is in flight.
  const [syncingRowKeys, setSyncingRowKeys] = useState<Record<string, true>>({})
  // Row keys the autocomplete has already fired for. A ref and not state: it must
  // never trigger a render, and it has to be read fresh inside an async callback.
  //
  // This set is what stands between one issuance and several real tickets at the
  // vendor. Every blur re-runs the same check, so without it a row that has its
  // minimum fields would fire again on each one.
  const autocompleteAttemptedRef = useRef<Set<string>>(new Set())
  const [autocompleteEnabled] = useAutocompletePreference()
  // True while the sweep below is pulling a row, so it never starts a second one.
  const [autocompleteRunning, setAutocompleteRunning] = useState(false)
  // Row index awaiting delete confirmation, driven by the Ctrl+Shift+Backspace shortcut.
  // Null means the confirm dialog is closed.
  const [pendingDeleteRow, setPendingDeleteRow] = useState<number | null>(null)
  // Lets the async account creation read the rows as they are when it resolves, not as
  // they were when it started.
  const rowsRef = useRef(rows)
  // Mirrors the interaction state for the stable (useCallback) handlers exposed through the
  // context: they read sel/sticky here instead of closing over the render's `interaction`,
  // so their identity stays stable and the context value doesn't change on every selection
  // move (which is what lets the memoized cells skip re-rendering).
  const interactionRef = useRef(interaction)
  // Mirrors syncingRowKeys for the stable callbacks, same reason as interactionRef:
  // onCellClick must not change identity every time a sync starts or ends.
  const syncingRowKeysRef = useRef(syncingRowKeys)
  // Mirrors the merged account list, same reason again: onCellClick needs to know whether
  // the row's account is exempt before it lets the redemption fee cell into edit mode.
  const accountsRef = useRef<AccountOption[]>([])
  // Undo/redo history. Each entry is the inverse patch (the field values as they were
  // before a change) plus the row it targets. Every value mutation funnels through
  // applyPatch, which records the inverse here; Ctrl+Z/Ctrl+Y replay entries between the
  // two stacks. Refs, not state: the history must not trigger re-renders and must be read
  // fresh inside the keydown handler without stale-closure surprises. historyModeRef tells
  // applyPatch which stack to feed while a replay is in flight (see runHistory).
  const undoStackRef = useRef<HistoryEntry[]>([])
  const redoStackRef = useRef<HistoryEntry[]>([])
  const historyModeRef = useRef<"record" | "undo" | "redo">("record")

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

  useEffect(() => {
    syncingRowKeysRef.current = syncingRowKeys
  }, [syncingRowKeys])

  useEffect(() => {
    accountsRef.current = accounts
  }, [accounts])

  useEffect(() => {
    rowsRef.current = rows
  }, [rows])

  // Publishes this grid's unsaved drafts for the next instance (see readCarriedDrafts).
  // A draft whose POST is in flight is left out on purpose: its create is already on the
  // way, and carrying it would let the next instance create the same issuance twice. The
  // snapshot is taken at unmount for the same reason - read later, the live rows would
  // still show a draft whose POST landed after this grid was gone.
  useEffect(() => {
    const unsavedDrafts = () =>
      rowsRef.current.filter(
        (row) =>
          row.isDraft && draftHasTypedContent(row) && !creatingKeysRef.current.has(row.key),
      )
    readCarriedDrafts = unsavedDrafts
    return () => {
      const snapshot = unsavedDrafts()
      readCarriedDrafts = () => snapshot
    }
  }, [])

  // F5 or closing the tab is the one exit a draft cannot survive, so ask first - the
  // same goes for a save still travelling, whose outcome would never be seen.
  useEffect(() => {
    function warnAboutUnsavedWork(event: BeforeUnloadEvent) {
      const hasUnsavedWork = rowsRef.current.some(
        (row) =>
          (row.isDraft && draftHasTypedContent(row)) || row.saveStatus === "saving",
      )
      if (!hasUnsavedWork) return
      event.preventDefault()
      // Still required by some browsers for the prompt to appear at all.
      event.returnValue = ""
    }

    window.addEventListener("beforeunload", warnAboutUnsavedWork)
    return () => window.removeEventListener("beforeunload", warnAboutUnsavedWork)
  }, [])

  useEffect(() => {
    interactionRef.current = interaction
  }, [interaction])

  useEffect(() => {
    if (interaction.editing) return

    // This effect also runs on mount, and the grid is remounted by the page on every
    // filter change - including each keystroke committed by the search box, which
    // lives outside the grid. Grabbing the catcher there would move the cursor out
    // of the field mid-word, so a text field the operator is typing in outside the
    // grid keeps the focus. Anything inside the grid (cells, the catcher itself)
    // still hands it over, which is what keyboard navigation depends on.
    const active = document.activeElement as HTMLElement | null
    const typingOutsideTheGrid =
      !!active &&
      !containerRef.current?.contains(active) &&
      (active.tagName === "INPUT" || active.tagName === "TEXTAREA" || active.isContentEditable)
    if (typingOutsideTheGrid) return

    // preventScroll: the catcher sits off-screen; focusing it must not yank
    // the viewport away from the cell the user is actually on.
    pasteCatcherRef.current?.focus({ preventScroll: true })
  }, [interaction.editing, interaction.sel.r, interaction.sel.c])

  // Keeps the selected cell in view as the user navigates. The catcher holds focus
  // off-screen, so the browser never scrolls to the active cell on its own; without this,
  // arrow/Tab navigation to a cell outside the viewport would leave it hidden. `nearest`
  // scrolls the minimum on both axes, and scroll-mt on the cells clears the sticky header.
  useEffect(() => {
    const cell = containerRef.current?.querySelector(
      `[data-cell="${interaction.sel.r}-${interaction.sel.c}"]`,
    )
    cell?.scrollIntoView({ block: "nearest", inline: "nearest" })
  }, [interaction.sel.r, interaction.sel.c])

  // Every handler below is wrapped in useCallback and reads volatile state via refs
  // (rowsRef/interactionRef) or stable setters, so their identity never changes across
  // renders. That keeps the memoized context value stable, which is the whole point of the
  // split: the cells only re-render when their own props change, not on every keystroke.
  const updateRow = useCallback((key: string, patch: Partial<IssuanceRow>) => {
    setRows((prev) => prev.map((row) => (row.key === key ? { ...row, ...patch } : row)))
  }, [])

  const removeRow = useCallback(
    (key: string) => {
      setRows((prev) => {
        const next = prev.filter((row) => row.key !== key)
        return next.length > 0 ? next : [makeDraftRow(currentUserId, draftAgencyId, recordingOnRef.current)]
      })
      setAccountClearedKeys((prev) => {
        if (!prev.has(key)) return prev
        const next = new Set(prev)
        next.delete(key)
        return next
      })
    },
    [currentUserId, draftAgencyId],
  )

  const flashSaveStatus = useCallback(
    (key: string, status: "success" | "error") => {
      // hasError outlives the flash: the dot resets to idle after 900ms, but a failed row
      // stays flagged red until a later save succeeds and clears it here.
      updateRow(key, { saveStatus: status, hasError: status === "error" })
      setTimeout(() => updateRow(key, { saveStatus: "idle" }), 900)
    },
    [updateRow],
  )

  // The backend fills defaults the client can't know (airline from the loyalty program,
  // the default agency, issuer from the JWT) and derives `status`, so a created row is
  // re-hydrated from the response instead of being patched with just the new id. The
  // draft's key is kept - issuanceToRow would swap in the backend id, remounting the row
  // and dropping the user's cell selection mid-typing.
  //
  // `pending` is whatever the operator typed while the POST was in flight. Those edits are
  // already on screen and about to be sent as a PATCH, but the response predates them -
  // re-hydrating from it alone put the old values back in those cells, and nothing but a
  // later SSE refresh would ever correct the display.
  const reconcileCreatedRow = useCallback(
    (key: string, issuance: Issuance, pending?: Partial<IssuanceRow>) => {
      setRows((prev) =>
        prev.map((row) =>
          row.key === key
            ? { ...issuanceToRow(issuance), ...pending, key, saveStatus: row.saveStatus }
            : row,
        ),
      )
    },
    [],
  )

  // persistRef holds the latest persist so the recursive replay (queued patch after a
  // create resolves) can call it without persist listing itself as a dependency.
  const persistRef = useRef<(row: IssuanceRow, patch: Partial<IssuanceRow>) => Promise<void>>(null)

  const persist = useCallback(
    async (row: IssuanceRow, patch: Partial<IssuanceRow>) => {
      const merged = { ...row, ...patch }

      if (row.isDraft) {
        if (creatingKeysRef.current.has(row.key)) {
          queuedPatchesRef.current.set(row.key, { ...queuedPatchesRef.current.get(row.key), ...patch })
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
          // `||` and not `??`: the locator cell writes an empty string to mean "no airline
          // locator", and a create has nothing to clear - the field is simply omitted.
          airline_locator: merged.airline_locator || undefined,
          latam_purchase_number: merged.latam_purchase_number || undefined,
          main_passenger_name: merged.main_passenger_name ?? undefined,
          origin_iata_code: merged.origin_iata_code ?? undefined,
          destination_iata_code: merged.destination_iata_code ?? undefined,
          taxes: merged.taxes ?? undefined,
          // Sent on the create too: a draft row is typed left to right, so the
          // observation is often already there when the fifth required field lands.
          observation: merged.observation ?? undefined,
          flight_date: merged.flight_date ?? undefined,
          // Sent on the create so a row typed for another day is born on it, rather than
          // being created today and moved a moment later.
          issued_on: merged.issued_on,
        }).finally(() => creatingKeysRef.current.delete(row.key))

        const queued = queuedPatchesRef.current.get(row.key)
        queuedPatchesRef.current.delete(row.key)

        if ("error" in result) {
          toast.error(result.error)
          flashSaveStatus(row.key, "error")
          // The row stays a draft holding those edits in local state, so the next cell
          // save retries the create with them merged in - replaying them as a PATCH here
          // would target an issuance that was never persisted.
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

      // `status` and the redemption fee are taken from the response: they are the displayed
      // fields the server derives on its own, and the fee moves on patches that never
      // mention it - clearing the cell recomputes it, and moving the row to an ordinary
      // account wipes it. A fuller merge is still avoided, since a late reply would clobber
      // a cell the user has edited since this save started.
      if (result.issuance) {
        updateRow(row.key, {
          status: result.issuance.status,
          redemption_fee_saved_cents: result.issuance.redemption_fee_saved_cents ?? null,
          redemption_fee_manual: result.issuance.redemption_fee_manual,
        })
      }

      flashSaveStatus(row.key, "success")
    },
    [updateRow, flashSaveStatus, reconcileCreatedRow],
  )
  // eslint-disable-next-line react-hooks/refs -- latest-ref read by callbacks memoized without persist in their deps
  persistRef.current = persist

  // Live updates. Every issuance mutation in the system announces itself on the SSE
  // stream, carrying only the affected id. The refresh below refetches the list with the
  // filters this page was rendered with, rather than deciding on the client whether the
  // changed row belongs on screen - those rules (the cancellation branch, the São Paulo
  // calendar day) live on the server and are not worth duplicating here.
  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // A correction from the bot can point at an account or an agency created after this page
  // rendered - the ALTERACAO flow conjures both from an unknown alias. The reference lists
  // are props of the Server Component and stay as they loaded, so without this the account
  // cell has no name to show and reads as empty until a reload.
  //
  // Nothing is fetched for it: GET /issuances already embeds account (with its nicknames)
  // and agency on every row, so the record is sitting in the response we just parsed.
  const adoptReferences = useCallback(
    (fresh: Issuance[]) => {
      setExtraAccounts((prev) =>
        absorb(prev, referenceData.accounts, fresh.map((issuance) => issuance.account)),
      )
      setExtraAgencies((prev) =>
        absorb(prev, referenceData.agencies, fresh.map((issuance) => issuance.agency)),
      )
      // Programs join the same trick for a different reason: an archived program is gone
      // from GET /loyalty-programs, and the Cia aérea cell resolves its label from that
      // list rather than from the object embedded in the row - so without this the column
      // would go blank on every issuance made on it.
      setExtraPrograms((prev) =>
        absorb(
          prev,
          referenceData.loyaltyPrograms,
          fresh.map((issuance) => issuance.loyalty_program),
        ),
      )
    },
    [referenceData.accounts, referenceData.agencies, referenceData.loyaltyPrograms],
  )

  const refreshFromServer = useCallback(async () => {
    let fresh: Issuance[]
    try {
      const res = await fetch(`/api/issuances?${filterQuery}`, { cache: "no-store" })
      const parsed = getIssuancesSchema.response.safeParse(await res.json())
      if (!res.ok || !parsed.success) return
      fresh = parsed.data.data ?? []
    } catch {
      // Same posture as the page's own fetches: a failed refresh leaves the grid showing
      // what it already has instead of blanking it out.
      return
    }

    adoptReferences(fresh)

    const { sel, editing } = interactionRef.current
    const selectedKey = rowsRef.current[sel.r]?.key ?? null
    const next = reconcileRows(rowsRef.current, fresh, editing ? selectedKey : null)
    setRows(next)

    // A row arriving above the selection shifts everything below it down. Follow the row
    // the user was actually on, or a remote insert would silently move the cursor onto a
    // different issuance.
    const movedTo = next.findIndex((row) => row.key === selectedKey)
    if (movedTo !== -1 && movedTo !== sel.r) {
      dispatch({ type: "MOVE", r: movedTo, c: sel.c, alsoEdit: editing })
    }
  }, [filterQuery, adoptReferences])

  // Coalesces a burst - a batch of issuances created back to back - into one refetch.
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

  // The redemption fee column measures an exemption an ordinary account does not have, so
  // every path that would write into it is closed there: click-to-edit, F2, type-to-replace,
  // Delete and paste. See acceptsRedemptionFee.
  const isLockedRedemptionFee = (c: number, row: IssuanceRow | undefined): boolean =>
    c === REDEMPTION_FEE_COLUMN_INDEX && row !== undefined && !acceptsRedemptionFee(row, accounts)

  const onCellClick = useCallback((r: number, c: number) => {
    activeCellSaveRef.current?.()
    activeCellSaveRef.current = null
    // A cancelled issuance is terminal: its cells select (so they can be read/copied) but
    // never enter edit mode - except the taxes column, see isLockedByCancellation. The
    // calculated column is likewise never editable.
    // A row with a Traveos fetch in flight is locked too: the fetch overwrites the
    // route and the passenger, so anything typed into it now would be replaced
    // seconds later without the operator seeing why.
    // The redemption fee column is locked on an ordinary account - see acceptsRedemptionFee.
    const row = rowsRef.current[r]
    const editable =
      c !== CALCULATED_COLUMN_INDEX &&
      !isLockedByCancellation(c, row) &&
      !(row && isRowSyncing(row, syncingRowKeysRef.current)) &&
      !(c === REDEMPTION_FEE_COLUMN_INDEX && row && !acceptsRedemptionFee(row, accountsRef.current))
    dispatch({ type: "CLICK_CELL", r, c, editable })
  }, [])

  // Applies a patch optimistically and persists it. Shared by the cell-save path and by
  // inline account creation, which lands its patch long after the cell has moved on -
  // both need the accountClearedKeys bookkeeping, or the account cell stays flagged red.
  const applyPatch = useCallback(
    (row: IssuanceRow, patch: Partial<IssuanceRow>) => {
      if (Object.keys(patch).length === 0) return

      // Record the inverse (the fields as they were before this patch) so Ctrl+Z can
      // replay it. `row` is the pre-patch state, so this is just its current values for
      // the keys being changed. During a replay, historyModeRef routes the inverse to the
      // opposite stack instead, which is what makes redo fall out for free.
      const inverse = Object.fromEntries(
        Object.keys(patch).map((key) => [key, row[key as keyof IssuanceRow]]),
      ) as Partial<IssuanceRow>
      if (historyModeRef.current === "record") {
        undoStackRef.current.push({ key: row.key, patch: inverse })
        if (undoStackRef.current.length > HISTORY_LIMIT) undoStackRef.current.shift()
        redoStackRef.current = [] // a fresh edit invalidates the redo stack
      } else if (historyModeRef.current === "undo") {
        redoStackRef.current.push({ key: row.key, patch: inverse })
      } else {
        undoStackRef.current.push({ key: row.key, patch: inverse })
      }

      if ("account_id" in patch) {
        setAccountClearedKeys((prev) => {
          const next = new Set(prev)
          if (patch.account_id === null) next.add(row.key)
          else next.delete(row.key)
          return next
        })
      }
      updateRow(row.key, patch)
      void persist(row, patch)
    },
    [updateRow, persist],
  )

  // Replays one step off the undo (or redo) stack. Re-applying through applyPatch both
  // persists the reversal and captures the current values onto the opposite stack, so
  // redo needs no special-casing. Entries whose row no longer exists (a deleted row -
  // structural changes are out of scope) are skipped so the stacks never dead-end. Reads
  // the current column from interactionRef, so it stays a stable identity like the rest.
  const runHistory = useCallback(
    (mode: "undo" | "redo") => {
      const stack = mode === "undo" ? undoStackRef : redoStackRef
      let entry = stack.current.pop()
      while (entry && !rowsRef.current.some((r) => r.key === entry!.key)) {
        entry = stack.current.pop()
      }
      if (!entry) return

      const row = rowsRef.current.find((r) => r.key === entry!.key)
      if (!row) return

      historyModeRef.current = mode
      applyPatch(row, entry.patch)
      historyModeRef.current = "record"

      // Move the selection onto the affected row (keeping the current column) so the change
      // the user just undid or redid is on screen rather than happening off in another row.
      const rowIndex = rowsRef.current.findIndex((r) => r.key === entry!.key)
      if (rowIndex >= 0) dispatch({ type: "SELECT_ONLY", r: rowIndex, c: interactionRef.current.sel.c })
    },
    [applyPatch],
  )

  const onSaveAndMove = useCallback(
    (r: number, patch: Partial<IssuanceRow>, direction: Direction | null) => {
      activeCellSaveRef.current = null
      const currentRows = rowsRef.current
      const row = currentRows[r]
      if (!row) return

      applyPatch(row, patch)

      if (direction === null) {
        dispatch({ type: "EXIT_EDIT_ONLY" })
        return
      }

      const { sel, sticky } = interactionRef.current
      const resolved = resolveNextCell(currentRows, sel, direction, 0, columnCountRef.current)
      if (resolved.appendRow) {
        setRows((prev) => [...prev, makeDraftRow(currentUserId, draftAgencyId, recordingOnRef.current)])
      }
      dispatch({ type: "MOVE", r: resolved.r, c: resolved.c, alsoEdit: sticky })
    },
    [applyPatch, currentUserId, draftAgencyId],
  )

  const resolveAccountEntryForRow = useCallback(
    async (rowKey: string, typed: string) => {
      const row = rowsRef.current.find((r) => r.key === rowKey)
      if (!row?.loyalty_program_id) return

      const programId = row.loyalty_program_id
      const { nickname, administratorName } = parseAccountEntry(typed)

      // The same narrowing buildAccountOptions applies: an archived account or one on another
      // program is not a choice, so matching either would point the row at an account the
      // dropdown never offered.
      const wanted = normalizeSearch(nickname)
      let existing: { accountId: string; nicknameId: string } | null = null
      for (const account of accountsRef.current) {
        if (account.deleted_at || account.loyalty_program_id !== programId) continue
        const match = account.nicknames.find((entry) => normalizeSearch(entry.nickname) === wanted)
        if (match) {
          existing = { accountId: account.id, nicknameId: match.id }
          break
        }
      }

      setPendingAccountLabels((prev) => ({ ...prev, [rowKey]: typed }))
      updateRow(rowKey, { saveStatus: "saving" })

      const clearPending = () =>
        setPendingAccountLabels((prev) => {
          const { [rowKey]: _removed, ...rest } = prev
          return rest
        })

      // Re-reads the row rather than closing over it: the awaits gave the operator time to keep
      // editing, and applyPatch hands the row it is given to persist() as the pre-patch base.
      const pointRowAt = (accountId: string, nicknameId: string | null) => {
        const current = rowsRef.current.find((r) => r.key === rowKey)
        if (!current) return
        applyPatch(current, { account_id: accountId, account_nickname_id: nicknameId })
      }

      // The alias is already registered, so this is a request to use that account - and, when a
      // name follows the dash, to give it an administrator. The row points at the account either
      // way: the alias matched, so it is the account the operator meant.
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
        pointRowAt(existing.accountId, existing.nicknameId)
        return
      }

      const result = await createAccountForIssuanceAction(nickname, programId, administratorName)

      clearPending()

      if ("error" in result) {
        toast.error(result.error)
        flashSaveStatus(rowKey, "error")
        return
      }

      setExtraAccounts((prev) => [...prev, result.account])
      pointRowAt(result.account.id, result.nicknameId)
    },
    [applyPatch, updateRow, flashSaveStatus, referenceData.people],
  )

  const onExitEdit = useCallback(() => {
    dispatch({ type: "ESC" })
  }, [])

  const registerActiveCellSave = useCallback((save: (() => void) | null) => {
    activeCellSaveRef.current = save
  }, [])

  const requestSaveActiveCell = useCallback(() => {
    activeCellSaveRef.current?.()
    activeCellSaveRef.current = null
  }, [])

  const onDeleteRow = useCallback(
    async (r: number) => {
      const row = rowsRef.current[r]
      if (!row) return

      if (row.isDraft) {
        removeRow(row.key)
        return
      }

      const result = await deleteIssuanceAction(row.id!)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      removeRow(row.key)
    },
    [removeRow],
  )

  const onCancelRow = useCallback(
    async (r: number, data: CancelIssuanceInput) => {
      const row = rowsRef.current[r]
      // A draft has no persisted id to cancel - the dialog only opens on real rows.
      if (!row || row.isDraft || !row.id) {
        return { error: "Emissão ainda não foi salva." }
      }

      const result = await cancelIssuanceAction(row.id, data)
      if ("error" in result) {
        return { error: result.error ?? "Erro ao cancelar emissão." }
      }
      // Local update mirrors persist(): flip the row to cancelled (turns it archived-grey,
      // swaps the cancel button for revert) and fold in the cancellation metadata from the
      // response so the status pill's hover card has it immediately, without a refetch.
      updateRow(row.key, {
        status: "cancelled",
        cancelled_at: result.issuance?.cancelled_at ?? null,
        cancelled_by_id: result.issuance?.cancelled_by_id ?? null,
        refunded_points: result.issuance?.refunded_points ?? null,
        refunded_taxes: result.issuance?.refunded_taxes ?? null,
        cancellation_fee: result.issuance?.cancellation_fee ?? null,
        cancellation_only: result.issuance?.cancellation_only ?? false,
      })
      return { success: true as const }
    },
    [updateRow],
  )

  const onUncancelRow = useCallback(
    async (r: number) => {
      const row = rowsRef.current[r]
      if (!row || row.isDraft || !row.id) {
        return { error: "Emissão ainda não foi salva." }
      }

      const result = await uncancelIssuanceAction(row.id)
      if ("error" in result) {
        return { error: result.error ?? "Erro ao reverter cancelamento." }
      }
      // Back to its derived status (complete/incomplete), editable again, with the
      // cancellation metadata cleared so the archived styling and hover card fall away.
      updateRow(row.key, {
        status: result.issuance?.status ?? "incomplete",
        cancelled_at: null,
        cancelled_by_id: null,
        refunded_points: null,
        refunded_taxes: null,
        cancellation_fee: null,
        // Reverting asserts the booking is live, which is the opposite of what an
        // estorno-only row claims; the backend clears the flag with it.
        cancellation_only: false,
      })
      return { success: true as const }
    },
    [updateRow],
  )

  // Pulls the itinerary from Traveos. Deliberately available on rows that are already
  // synced: pressing it there is not a retry, it is "the flight changed, fetch the new
  // data", which is the only way a reschedule reaches the system.
  const onSyncRowWithTraveos = useCallback(
    async (r: number) => {
      const row = rowsRef.current[r]
      if (!row || row.isDraft || !row.id) {
        return { error: "Emissão ainda não foi salva." }
      }
      if (syncingRowKeys[row.key]) {
        return { error: "Já estou puxando os dados dessa emissão." }
      }
      if (row.traveos_syncing_at) {
        return { error: "Essa emissão já está sendo puxada da Traveos." }
      }

      setSyncingRowKeys((prev) => ({ ...prev, [row.key]: true }))
      try {
        const result = await syncIssuanceWithTraveosAction(row.id)
        if ("error" in result) {
          return { error: result.error ?? "Erro ao puxar dados da Traveos." }
        }

        // The sync rewrites passengers, flights, route and trip type, so the fields the
        // grid shows are taken from the response rather than guessed at. Only the ones
        // this grid renders are applied - the itinerary itself lives on the ticket page.
        const issuance = result.issuance
        updateRow(row.key, {
          status: issuance?.status ?? "complete",
          traveos_synced_at: issuance?.traveos_synced_at ?? new Date().toISOString(),
          // The server clears its marker as the sync ends; carrying a stale one here
          // would keep the row locked until the next refresh landed.
          traveos_syncing_at: null,
          main_passenger_name: issuance?.main_passenger_name ?? row.main_passenger_name,
          origin_iata_code: issuance?.origin_iata_code ?? row.origin_iata_code,
          destination_iata_code: issuance?.destination_iata_code ?? row.destination_iata_code,
          flight_date: issuance?.flights?.[0]?.departure_at.slice(0, 10) ?? row.flight_date,
        })
        return { success: true as const }
      } finally {
        setSyncingRowKeys((prev) => {
          const next = { ...prev }
          delete next[row.key]
          return next
        })
      }
    },
    [syncingRowKeys, updateRow],
  )

  // Sweeps the rows on screen and fires the same sync the button does, on the first
  // one that has everything its airline needs. It runs on every change to the list -
  // so it catches both the row just completed by hand and the rows that were already
  // waiting when the page loaded, which the old "right after this row saved" trigger
  // never looked at.
  //
  // Strictly one at a time. A sync holds the vendor for minutes; letting a full grid
  // start dozens at once would queue every operator behind them. When one finishes
  // this effect runs again and picks up the next.
  useEffect(() => {
    if (!autocompleteEnabled || autocompleteRunning) return

    const index = rowsRef.current.findIndex((row) =>
      isAutocompleteCandidate(row, referenceData.airlines, autocompleteAttemptedRef.current),
    )
    if (index === -1) return

    autocompleteAttemptedRef.current.add(rowsRef.current[index].key)
    setAutocompleteRunning(true)
    void onSyncRowWithTraveos(index)
      .then((result) => {
        if ("error" in result) {
          // The row stays in the attempted set on purpose: whatever went wrong will go
          // wrong again, and the operator has an explicit button for when they have
          // actually changed something.
          toast.error(result.error)
          return
        }
        toast.success("Dados da Traveos preenchidos.")
      })
      .finally(() => setAutocompleteRunning(false))
  }, [
    rows,
    autocompleteEnabled,
    autocompleteRunning,
    referenceData.airlines,
    onSyncRowWithTraveos,
  ])

  // Keyboard entry point for deleting the active row. An untouched empty draft has nothing
  // to lose, so it is dropped without a prompt; anything else opens the confirm dialog.
  function requestRowDelete(r: number) {
    const row = rows[r]
    if (!row) return
    if (isUntouchedDraft(row)) {
      void onDeleteRow(r)
      return
    }
    setPendingDeleteRow(r)
  }

  function handleAddRow() {
    const lastRow = rows[rows.length - 1]
    if (lastRow && isUntouchedDraft(lastRow)) {
      dispatch({ type: "CLICK_CELL", r: rows.length - 1, c: 0, editable: true })
      return
    }

    const newIndex = rows.length
    setRows((prev) => [...prev, makeDraftRow(currentUserId, draftAgencyId, recordingOnRef.current)])
    dispatch({ type: "CLICK_CELL", r: newIndex, c: 0, editable: true })
  }

  function handleContainerKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (interaction.editing) return

    // Power-user jumps and row ops, handled before the single-step arrows/Enter below so
    // the modifier combinations win over their unmodified counterparts.
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
      e.preventDefault()
      handleAddRow()
      return
    }
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === "Backspace" || e.key === "Delete")) {
      e.preventDefault()
      requestRowDelete(interaction.sel.r)
      return
    }
    if (
      e.key === "Home" ||
      e.key === "End" ||
      ((e.ctrlKey || e.metaKey) &&
        (e.key === "ArrowUp" ||
          e.key === "ArrowDown" ||
          e.key === "ArrowLeft" ||
          e.key === "ArrowRight"))
    ) {
      e.preventDefault()
      const lastRow = Math.max(rows.length - 1, 0)
      const lastCol = columnCount - 1
      const toCorner = e.ctrlKey || e.metaKey // Ctrl+Home/End jump to the grid corner
      let r = interaction.sel.r
      let c = interaction.sel.c
      if (e.key === "Home") {
        c = 0
        if (toCorner) r = 0
      } else if (e.key === "End") {
        c = lastCol
        if (toCorner) r = lastRow
      } else if (e.key === "ArrowUp") r = 0
      else if (e.key === "ArrowDown") r = lastRow
      else if (e.key === "ArrowLeft") c = 0
      else if (e.key === "ArrowRight") c = lastCol
      dispatch({ type: "SELECT_ONLY", r, c })
      return
    }

    // Enter moves down and Shift+Enter up, like a spreadsheet; editing a cell is F2,
    // a click, or typing (type-to-replace). Handled with the arrows so it works from any
    // column, including the read-only calculated one.
    if (
      e.key === "ArrowDown" ||
      e.key === "ArrowUp" ||
      e.key === "ArrowLeft" ||
      e.key === "ArrowRight" ||
      e.key === "Enter"
    ) {
      e.preventDefault()
      const dr =
        e.key === "ArrowDown" || (e.key === "Enter" && !e.shiftKey)
          ? 1
          : e.key === "ArrowUp" || (e.key === "Enter" && e.shiftKey)
            ? -1
            : 0
      const dc = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0
      const next = clampMove(rows.length, interaction.sel, dr, dc, columnCount)
      dispatch({ type: "SELECT_ONLY", r: next.r, c: next.c })
      return
    }

    if (e.key === "Tab") {
      e.preventDefault()
      const direction: Direction = e.shiftKey ? "tab-left" : "tab-right"
      const resolved = resolveNextCell(rows, interaction.sel, direction, 0, columnCount)
      if (resolved.appendRow) setRows((prev) => [...prev, makeDraftRow(currentUserId, draftAgencyId, recordingOnRef.current)])
      dispatch({ type: "MOVE", r: resolved.r, c: resolved.c, alsoEdit: interaction.sticky })
      return
    }

    // Undo/redo are grid-wide, not tied to the active cell, so they sit before the
    // calculated-column guard below (which only gates per-cell editing actions).
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "z" && !e.shiftKey) {
      e.preventDefault()
      runHistory("undo")
      return
    }
    if (
      (e.ctrlKey || e.metaKey) &&
      (e.key.toLowerCase() === "y" || (e.shiftKey && e.key.toLowerCase() === "z"))
    ) {
      e.preventDefault()
      runHistory("redo")
      return
    }

    // Copying the row as an EMISSAO message, same as the copy menu in RowActions. Like
    // undo/redo above, these act on the row rather than on the active cell, so they sit
    // before the calculated-column guard - anything below it is dead on that column.
    if ((e.ctrlKey || e.metaKey) && e.altKey && (e.key.toLowerCase() === "c" || e.code === "KeyC")) {
      e.preventDefault()
      const row = rows[interaction.sel.r]
      // The merged lists (accounts/agencies/programs learned during this session), the
      // same ones the context hands the copy menu.
      if (row) {
        void copyEmissaoText(
          buildEmissaoResumo(row, {
            loyaltyPrograms,
            airlines: referenceData.airlines,
            accounts,
            agencies,
            people: referenceData.people,
          }),
          "resumo",
        )
      }
      return
    }
    if ((e.ctrlKey || e.metaKey) && e.altKey && (e.key.toLowerCase() === "x" || e.code === "KeyX")) {
      e.preventDefault()
      const row = rows[interaction.sel.r]
      if (row) void copyEmissaoText(buildCalculo(row), "cálculo")
      return
    }

    const isCalculated = interaction.sel.c === CALCULATED_COLUMN_INDEX
    if (isCalculated) return

    // A cancelled row is read-only apart from its taxes: block every path that starts an
    // edit or mutates a cell (F2, Delete, replace-on-type below). Navigation and Ctrl+C stay
    // allowed - hence these guards are per-branch rather than a single early return.
    const cellIsLockedByCancellation = isLockedByCancellation(
      interaction.sel.c,
      rows[interaction.sel.r],
    )

    if (e.key === "F2") {
      if (cellIsLockedByCancellation) return
      if (isLockedRedemptionFee(interaction.sel.c, rows[interaction.sel.r])) return
      e.preventDefault()
      dispatch({ type: "ENTER_EDIT" })
      return
    }

    if (e.key === "Delete") {
      if (cellIsLockedByCancellation) return
      const patch = clearPatchForColumn(interaction.sel.c)
      const row = rows[interaction.sel.r]
      if (isLockedRedemptionFee(interaction.sel.c, row)) return
      if (patch && row) {
        e.preventDefault()
        applyPatch(row, patch)
      }
      return
    }

    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "c") {
      e.preventDefault()
      void copyActiveCell()
      return
    }

    // Ctrl+V is not handled here: it arrives as a native `paste` event on the
    // focused catcher textarea (see handlePaste), which reads the clipboard
    // synchronously without a permission prompt.

    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      if (cellIsLockedByCancellation) return
      if (isLockedRedemptionFee(interaction.sel.c, rows[interaction.sel.r])) return
      dispatch({ type: "TYPE_REPLACE", char: e.key })
    }
  }

  // Copies the value the user actually sees (a combobox shows a name, not its id),
  // read from the rendered read-view via the cell's data-cell marker. The empty
  // placeholder ("—") copies as an empty string.
  async function copyActiveCell() {
    const node = containerRef.current?.querySelector(
      `[data-cell="${interaction.sel.r}-${interaction.sel.c}"]`,
    )
    const text = (node?.textContent ?? "").trim()
    const copyable = text === "—" ? "" : text
    try {
      await navigator.clipboard.writeText(copyable)
      toast.success("Célula copiada para a área de transferência")
    } catch {
      toast.error("Não foi possível copiar a célula")
    }
  }

  // Pastes into the active cell by entering edit mode with the clipboard text
  // pre-filled (pendingChar carries multi-character text just as well as a single
  // typed char), so each cell's own parser decides the value on commit. Fires
  // from the catcher textarea's native paste event, so it never touches the
  // permission-gated navigator.clipboard.readText().
  function handlePaste(e: React.ClipboardEvent<HTMLTextAreaElement>) {
    // Only meaningful outside edit mode: while editing, the cell's own input has
    // focus and handles its native paste directly.
    if (interaction.editing) return
    if (interaction.sel.c === CALCULATED_COLUMN_INDEX) return
    // A cancelled row is read-only apart from its taxes - a paste must not seed an edit
    // into any other cell of it.
    if (isLockedByCancellation(interaction.sel.c, rows[interaction.sel.r])) return
    if (isLockedRedemptionFee(interaction.sel.c, rows[interaction.sel.r])) return
    e.preventDefault()
    const text = e.clipboardData.getData("text/plain")
    if (text) dispatch({ type: "TYPE_REPLACE", char: text })
  }

  // What the selected row still needs before the autocomplete will pull from Traveos.
  // Keyed off the selection, so it follows the operator down the grid and clears itself
  // the moment they leave the row - it is a hint about the row being worked on, not a
  // validation state the row carries around.
  const autocompleteHint = useMemo(
    () => autocompleteHintFor(rows[interaction.sel.r], referenceData.airlines, autocompleteEnabled),
    [rows, interaction.sel.r, referenceData.airlines, autocompleteEnabled],
  )
  const missingColumnReasons = useMemo(
    () =>
      autocompleteHint.kind === "missing"
        ? new Map(autocompleteHint.requirements.map((req) => [req.column, req.reason]))
        : new Map<number, string>(),
    [autocompleteHint],
  )

  // The context carries only referentially-stable data: reference lists, the memoized
  // callbacks above and pendingAccountLabels (which changes only during inline account
  // creation). The volatile per-cell state (row/isEditing/pendingChar) is passed as props
  // in the render loop instead, so moving the selection doesn't invalidate this value and
  // the memoized cells can skip re-rendering. Depends on the individual reference arrays,
  // not the `referenceData` rest object, which is a fresh object every render.
  const contextValue = useMemo<GridContextValue>(
    () => ({
      loyaltyPrograms,
      airlines: referenceData.airlines,
      // Override the props: both include the records this session picked up after the
      // page rendered - created inline, or adopted from a refetched issuance.
      accounts,
      agencies,
      users: referenceData.users,
      people: referenceData.people,
      airports,
      onCellClick,
      onSaveAndMove,
      onExitEdit,
      registerActiveCellSave,
      requestSaveActiveCell,
      onDeleteRow,
      onCancelRow,
      onUncancelRow,
      onSyncRowWithTraveos,
      resolveAccountEntryForRow,
      pendingAccountLabels,
      syncingRowKeys,
    }),
    [
      loyaltyPrograms,
      referenceData.airlines,
      accounts,
      agencies,
      referenceData.users,
      referenceData.people,
      airports,
      onCellClick,
      onSaveAndMove,
      onExitEdit,
      registerActiveCellSave,
      requestSaveActiveCell,
      onDeleteRow,
      onCancelRow,
      onUncancelRow,
      onSyncRowWithTraveos,
      resolveAccountEntryForRow,
      pendingAccountLabels,
      syncingRowKeys,
    ],
  )

  return (
    <GridProvider value={contextValue}>
      <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-2 bg-background py-2">
        {filters}
        <div className="ml-auto flex items-center gap-2">
          {/* Sits here rather than in the filter bar (which the page owns and passes in as
              `filters`), and lands next to the Autocompletar toggle because that toggle is
              ml-auto at the filter bar's right edge. No red headers in this case: the row
              will never autocomplete, so pointing at columns would be a false promise. */}
          {autocompleteHint.kind === "unsupported" && (
            <span className="flex items-center gap-1.5 text-xs text-amber-600">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              A {autocompleteHint.airlineName} não está integrada à Traveos — esta emissão não
              autocompleta.
            </span>
          )}
          {autocompleteHint.kind === "imported" && (
            <span className="flex items-center gap-1.5 text-xs text-amber-600">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              Emissão importada da planilha: a Traveos já foi consultada e não tem essa reserva.
              Os dados do voo precisam ser preenchidos à mão.
            </span>
          )}
          <Popover>
            <PopoverTrigger asChild>
              <Button type="button" variant="outline">
                <Keyboard className="h-4 w-4" />
                Atalhos
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-80">
              <p className="mb-2 text-sm font-medium">Atalhos de teclado</p>
              <dl className="space-y-1">
                {SHORTCUTS.map((shortcut) => (
                  <div
                    key={shortcut.keys}
                    className="flex items-baseline justify-between gap-3 text-xs"
                  >
                    <dt className="text-muted-foreground">{shortcut.label}</dt>
                    <dd className="shrink-0 font-mono text-[11px]">{shortcut.keys}</dd>
                  </div>
                ))}
              </dl>
            </PopoverContent>
          </Popover>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline">
                <FileSpreadsheet className="h-4 w-4" />
                Exportar
                <ChevronDown className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            {/* Native downloads: the backend sets the filename via Content-Disposition, and
                filterQuery carries the same filters the page was rendered with. Both layouts
                stay on offer because some operators still reconcile against the old sheet. */}
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <a href={`/api/issuances/export?${filterQuery}`} download>
                  <FileSpreadsheet />
                  Planilha de controle
                </a>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <a href={`/api/issuances/export?${filterQuery ? `${filterQuery}&` : ""}layout=legacy`} download>
                  <FileSpreadsheet />
                  Planilha Emissões Gerais (antiga)
                </a>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          {/* Which day the next rows belong to. Sits beside "Nova emissão" because it is
              part of the same act - the rows this button creates are recorded on it. */}
          <label
            className={cn(
              "flex items-center gap-2 rounded-md border px-2 py-1 text-sm",
              backdating && "border-amber-400 bg-amber-50 text-amber-900",
            )}
            title="O dia em que as próximas emissões aconteceram"
          >
            <CalendarDays className="h-4 w-4 shrink-0" />
            <span className="text-xs text-muted-foreground">Lançando em</span>
            <DateInput
              value={recordingOn}
              onChange={(value) => setRecordingOn(value || todayISO())}
              className="w-36 [&_input]:h-7 [&_input]:border-0 [&_input]:bg-transparent [&_input]:px-0 [&_input]:shadow-none [&_input]:focus-visible:ring-0"
            />
            {backdating && (
              <button
                type="button"
                onClick={() => setRecordingOn(todayISO())}
                className="text-xs underline underline-offset-2"
              >
                hoje
              </button>
            )}
          </label>
          {/* Hidden by default: the date is normally today, and the control above says so.
              It cannot be hidden while backdating - see issuedOnVisible. */}
          <Button
            type="button"
            variant={issuedOnVisible ? "secondary" : "outline"}
            onClick={toggleIssuedOnColumn}
            disabled={backdating}
            title={
              backdating
                ? "Enquanto estiver lançando em outra data, a coluna fica visível"
                : "Mostrar ou esconder a coluna Data da emissão"
            }
          >
            <CalendarDays className="h-4 w-4" />
            Data da emissão
          </Button>
          <Button type="button" onClick={handleAddRow}>
            <Plus className="h-4 w-4" />
            Nova emissão
          </Button>
        </div>
      </div>
      <div
        ref={containerRef}
        onKeyDown={handleContainerKeyDown}
        // Bounded scroll region so the header can stick to its top (a sticky header needs a
        // scroll ancestor with a fixed height) and long lists scroll inside the grid.
        className="relative max-h-[calc(100vh-13rem)] overflow-auto rounded-md border outline-none"
      >
        {/* Off-screen focus holder: keydown and paste events from it bubble to the
            container's onKeyDown/handlePaste. Kept empty (onInput clears any char
            that type-to-replace lets through) so it never accumulates text. */}
        <textarea
          ref={pasteCatcherRef}
          aria-hidden="true"
          tabIndex={-1}
          onPaste={handlePaste}
          onInput={(e) => {
            e.currentTarget.value = ""
          }}
          className="pointer-events-none absolute left-0 top-0 h-px w-px resize-none opacity-0"
        />
        <table className="w-full caption-bottom text-sm">
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="sticky top-0 z-10 w-8 bg-background p-1 text-center">#</TableHead>
              {/* Sliced rather than mapped whole: the last label is the issuance date,
                  which is only on screen when the column is. */}
              {COLUMN_LABELS.slice(0, columnCount).map((label, c) => {
                // Red means "the autocomplete is still waiting on this one", and it clears
                // as each is filled - so the header row doubles as a live checklist for
                // the row being typed.
                const missingReason = missingColumnReasons.get(c)
                return (
                  <TableHead
                    key={label}
                    title={missingReason}
                    className={cn(
                      "sticky top-0 z-10 whitespace-nowrap bg-background",
                      c === CALCULATED_COLUMN_INDEX && "text-right",
                      missingReason && "text-destructive",
                    )}
                  >
                    {label}
                  </TableHead>
                )
              })}
              <TableHead className="sticky top-0 z-10 w-8 bg-background p-1" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 && (
              <TableRow className="hover:bg-transparent">
                <TableCell
                  colSpan={columnCount + 2}
                  className="h-32 text-center text-sm text-muted-foreground"
                >
                  Nenhuma emissão encontrada para o período e filtros selecionados.
                  <br />
                  Ajuste os filtros ou clique em{" "}
                  <span className="font-medium">Nova emissão</span> para cadastrar.
                </TableCell>
              </TableRow>
            )}
            {rows.map((row, r) => (
              <TableRow
                key={row.key}
                className={cn(
                  "group/row",
                  // Cancelled reads as archived/terminal (muted grey), keeping red exclusively
                  // for a failed save so the two states are never confused.
                  row.status === "cancelled" && "bg-muted/40 text-muted-foreground hover:bg-muted/40",
                  row.hasError && "bg-red-50 hover:bg-red-50",
                  // Locked while Traveos is being read - see onCellClick.
                  isRowSyncing(row, syncingRowKeys) && "bg-sky-50/60 hover:bg-sky-50/60",
                )}
              >
                <TableCell
                  className={cn(
                    "relative w-8 p-1 text-center text-xs text-muted-foreground tabular-nums",
                    row.hasError && "border-l-4 border-l-red-600",
                  )}
                >
                  {r + 1}
                  <span className="absolute right-0.5 top-0.5">
                    <AutosaveDot status={row.saveStatus} />
                  </span>
                </TableCell>
                {Array.from({ length: columnCount }, (_, c) => {
                  const accountNeedsPicking =
                    c === ACCOUNT_COLUMN_INDEX && accountClearedKeys.has(row.key)
                  const isSelected = interaction.sel.r === r && interaction.sel.c === c
                  const isEditing = isSelected && interaction.editing
                  return (
                    <TableCell
                      key={c}
                      // Marks the cell for Ctrl+C, which copies its rendered text.
                      data-cell={`${r}-${c}`}
                      // Background rather than a ring or border: the selection ring below
                      // already owns --tw-ring-color on this cell, and two competing
                      // ring-* utilities resolve by their order in the generated CSS, not
                      // by the order they appear here.
                      className={cn(
                        // scroll-mt clears the sticky header when scrollIntoView lands here.
                        "scroll-mt-9 whitespace-normal border-l p-0 first:border-l-0",
                        // The only free-text column with no length of its own. Bounded so
                        // one long observation cannot widen the table for every other row -
                        // the cell truncates and the full text is on the hover title.
                        c === OBSERVATION_COLUMN_INDEX && "max-w-64",
                        accountNeedsPicking && "bg-red-100",
                        // Strike through the locator of a cancelled row - a "voided" cue on the
                        // cell that identifies the issuance.
                        row.status === "cancelled" && c === LOCATOR_COLUMN_INDEX && "line-through",
                        isSelected && "ring-2 ring-inset ring-primary",
                      )}
                      title={
                        accountNeedsPicking
                          ? row.isDraft
                            ? "Selecione uma conta — a emissão não será salva sem ela."
                            : "Selecione uma conta — a emissão ainda está vinculada à conta do programa anterior."
                          : undefined
                      }
                    >
                      {/* Volatile state as props: `row` (stable ref unless this row was
                          edited), whether this cell is editing, and pendingChar only while
                          it is. Every other cell gets identical props across a selection
                          move, so its memoized GridCell skips re-rendering. */}
                      <GridCell
                        r={r}
                        c={c}
                        row={row}
                        isEditing={isEditing}
                        pendingChar={isEditing ? interaction.pendingChar : null}
                      />
                    </TableCell>
                  )
                })}
                <TableCell className="p-1">
                  <RowActions r={r} row={row} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </table>
      </div>

      <RedemptionFeeSummary rows={rows} />

      {/* Confirmation for the keyboard row-delete (Ctrl+Shift+Backspace). The per-row
          trash button in RowActions has its own trigger-based dialog; this one is opened
          from the keyboard, so it is controlled and returns focus to the catcher on close. */}
      <AlertDialog
        open={pendingDeleteRow !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDeleteRow(null)
        }}
      >
        <AlertDialogContent
          onCloseAutoFocus={(e) => {
            e.preventDefault()
            pasteCatcherRef.current?.focus({ preventScroll: true })
          }}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>Deletar emissão</AlertDialogTitle>
            <AlertDialogDescription>Você realmente deseja deletar essa emissão?</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-red-300 text-red-600 hover:bg-red-50 hover:text-red-700">
              Não
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pendingDeleteRow !== null) void onDeleteRow(pendingDeleteRow)
                setPendingDeleteRow(null)
              }}
            >
              Sim, deletar
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </GridProvider>
  )
}
