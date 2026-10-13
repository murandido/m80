"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import Link from "next/link"
import {
  ArrowLeft,
  Calculator,
  ChevronsUpDown,
  CloudDownload,
  Copy,
  Loader2,
  MessageSquareText,
  RotateCcw,
  Ticket,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
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
import { cn } from "@/lib/utils"
import { copyToClipboard } from "@/lib/clipboard"
import { formatCurrencyBRL } from "@/lib/currency"
import { normalizeSearch } from "@/lib/search"
import {
  GridProvider,
  type GridContextValue,
} from "@/components/issuances/grid-context"
import { CancelIssuanceButton } from "@/components/issuances/cancel-issuance-dialog"
import { AdjustmentsSection } from "@/components/issuances/board/adjustments-section"
import { copyEmissaoText } from "@/components/issuances/copy-emissao-menu"
import {
  accountFallbackLabel,
  buildCalculo,
  buildEmissaoResumo,
  issuanceTotalCents,
} from "@/components/issuances/emissao-message"
import { parseAccountEntry } from "@/components/issuances/account-entry"
import { acceptsRedemptionFee } from "@/components/issuances/redemption-fee"
import { isRowSyncing, type IssuanceRow } from "@/components/issuances/types"
import {
  ComboField,
  DateField,
  DisplayField,
  FieldGroup,
  NumberField,
  TextField,
  focusField,
  type ComboOption,
  type FieldTint,
} from "@/components/issuances/board/board-fields"
import {
  airlineAutocompletes,
  type IssuanceBoardState,
} from "@/components/issuances/board/use-issuance-board"
import type { AccountOption } from "@/schemas/account"
import type { Airport } from "@/schemas/airport"
import type { IssuanceStatus } from "@/schemas/issuance"
import type { PersonOption } from "@/schemas/person"

// The blue of a lookup in flight (design image 7): the fields it was asked with, while the
// airline answers.
const LOOKUP_TINT: FieldTint = { color: "#3b82f6", strong: true }

const STATUS_LABELS: Record<IssuanceStatus, string> = {
  incomplete: "Incompleto",
  complete: "Completo",
  cancelled: "Cancelado",
}

const MAX_AIRPORT_OPTIONS = 50

function airportOptions(airports: Airport[], typed: string): ComboOption[] {
  const query = normalizeSearch(typed)
  const matches = query
    ? airports
        .filter((airport) =>
          [
            airport.iata_code,
            airport.name,
            airport.city,
            airport.state,
            airport.country,
          ].some((value) =>
            value ? normalizeSearch(value).includes(query) : false,
          ),
        )
        .sort((a, b) => {
          const aStarts = normalizeSearch(a.iata_code).startsWith(query)
          const bStarts = normalizeSearch(b.iata_code).startsWith(query)
          if (aStarts === bStarts) return 0
          return aStarts ? -1 : 1
        })
    : airports
  return matches.slice(0, MAX_AIRPORT_OPTIONS).map((airport) => ({
    key: airport.iata_code,
    label: airport.iata_code,
    hint: airport.city ?? airport.name,
  }))
}

type AccountChoice = ComboOption & {
  accountId: string
  nicknameId: string | null
}

// One option per alias, narrowed to the row's program - the grid's buildAccountOptions.
function accountOptions(
  accounts: AccountOption[],
  people: PersonOption[],
  programId: string | null,
): AccountChoice[] {
  return accounts
    .filter((account) => !account.deleted_at)
    .filter((account) => !programId || account.loyalty_program_id === programId)
    .flatMap((account): AccountChoice[] =>
      account.nicknames.length > 0
        ? account.nicknames.map((nickname) => ({
            key: nickname.id,
            label: nickname.nickname,
            accountId: account.id,
            nicknameId: nickname.id,
          }))
        : [
            {
              key: account.id,
              label: accountFallbackLabel(account, people),
              accountId: account.id,
              nicknameId: null,
            },
          ],
    )
}

interface IssuancePanelProps {
  board: IssuanceBoardState
  row: IssuanceRow
  number: number
  // Focus the first field on open: a new issuance is typed at once, an existing one is read.
  focusFirstField: boolean
  onMinimize: () => void
  deleteRequested: boolean
  onDeleteRequestedChange: (open: boolean) => void
}

export function IssuancePanel({
  board,
  row,
  number,
  focusFirstField,
  onMinimize,
  deleteRequested,
  onDeleteRequestedChange,
}: IssuancePanelProps) {
  const formRef = useRef<HTMLDivElement>(null)
  const headingRef = useRef<HTMLHeadingElement>(null)
  const {
    airlines,
    loyaltyPrograms,
    accounts,
    agencies,
    users,
    people,
    airports,
    applyPatch,
    autocompleteEnabled,
    setAutocompleteEnabled,
  } = board

  const program = loyaltyPrograms.find(
    (candidate) => candidate.id === row.loyalty_program_id,
  )
  const airline = airlines.find((candidate) => candidate.id === row.airline_id)
  const autocompletes = airlineAutocompletes(airline)
  const lookingUp = Boolean(board.lookingUpKeys[row.key])
  const syncing = isRowSyncing(row, board.syncingRowKeys)
  const cancelled = row.status === "cancelled"

  // Which form is on screen. The short one carries only what the lookup needs; it is the
  // starting point for a new issuance while the autocomplete is on, and makes no sense for
  // an airline nothing can look up.
  const lookupViewPossible =
    autocompleteEnabled && !cancelled && (!airline || autocompletes)
  const [preferLookupView, setPreferLookupView] = useState(row.isDraft)
  const showLookupView = lookupViewPossible && preferLookupView

  // A lookup that just came back with the itinerary opens the full form, which is where
  // what it filled in is shown.
  const [wasLookingUp, setWasLookingUp] = useState(lookingUp)
  if (wasLookingUp !== lookingUp) {
    setWasLookingUp(lookingUp)
    if (!lookingUp && row.destination_iata_code) setPreferLookupView(false)
  }

  // The second locator is shown when the airline is not the program's own, and also on
  // demand - "/" in the program's locator - for a booking that has one anyway.
  const [secondLocatorRequested, setSecondLocatorRequested] = useState(false)
  const nonDefaultAirline = Boolean(
    program && airline && airline.id !== program.default_airline_id,
  )
  const showSecondLocator =
    nonDefaultAirline ||
    secondLocatorRequested ||
    Boolean(row.airline_locator?.trim())
  const showPurchaseNumber =
    Boolean(airline?.requires_purchase_number) ||
    Boolean(row.latam_purchase_number?.trim())

  useEffect(() => {
    if (focusFirstField) focusField(formRef.current, "program")
    else headingRef.current?.focus({ preventScroll: true })
    // Once per opened issuance: switching cards is a new panel instance.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const programTint: FieldTint | null = program
    ? { color: program.color }
    : null
  const airlineColor = airline?.brand_color ?? program?.color
  const airlineTint: FieldTint | null =
    airline && airlineColor ? { color: airlineColor } : null
  // The fields a lookup was asked with turn blue and freeze while it runs: what it answers
  // is about those values, and a change mid-flight would be overwritten by the answer.
  const lookupTint = (base: FieldTint | null) =>
    lookingUp ? LOOKUP_TINT : base
  const itineraryTint = syncing || lookingUp ? LOOKUP_TINT : null
  const lookupLocked = cancelled || lookingUp
  const itineraryLocked = cancelled || syncing || lookingUp

  const programOptions = useMemo<ComboOption[]>(
    () =>
      loyaltyPrograms
        .filter((candidate) => !candidate.deleted_at)
        .map((candidate) => ({
          key: candidate.id,
          label: candidate.name,
          hint: candidate.code,
        })),
    [loyaltyPrograms],
  )
  const airlineOptions = useMemo<ComboOption[]>(
    () =>
      airlines.map((candidate) => ({
        key: candidate.id,
        label: candidate.name,
        hint: candidate.code,
      })),
    [airlines],
  )
  const accountChoices = useMemo(
    () => accountOptions(accounts, people, row.loyalty_program_id),
    [accounts, people, row.loyalty_program_id],
  )
  const selectedAccountKey = row.account_nickname_id ?? row.account_id
  // An archived account still labels the issuance made on it, though it is not offered.
  const accountDisplay =
    board.pendingAccountLabels[row.key] ??
    accountChoices.find((option) => option.key === selectedAccountKey)?.label ??
    (() => {
      const account = accounts.find(
        (candidate) => candidate.id === row.account_id,
      )
      if (!account) return ""
      return (
        account.nicknames.find((entry) => entry.id === row.account_nickname_id)
          ?.nickname ?? accountFallbackLabel(account, people)
      )
    })()

  const patch = (fields: Partial<IssuanceRow>) => applyPatch(row.key, fields)

  // A program change carries its default airline, and drops the account: an account
  // belongs to one program.
  function pickProgram(programId: string) {
    const chosen = loyaltyPrograms.find(
      (candidate) => candidate.id === programId,
    )
    if (!chosen) return
    const changes: Partial<IssuanceRow> = {
      loyalty_program_id: chosen.id,
      airline_id: chosen.default_airline_id,
    }
    if (chosen.id !== row.loyalty_program_id && row.account_id !== null) {
      changes.account_id = null
      changes.account_nickname_id = null
    }
    patch(changes)
  }

  function findByCodeOrName<T extends { code: string; name: string }>(
    list: T[],
    typed: string,
  ) {
    const query = normalizeSearch(typed)
    return (
      list.find((candidate) => normalizeSearch(candidate.code) === query) ??
      list.find((candidate) => normalizeSearch(candidate.name) === query)
    )
  }

  const accountCleared = board.accountClearedKeys.has(row.key)
  const accountInvalid = accountCleared && row.account_id === null

  const total = issuanceTotalCents(row)
  // What the sale is worth once the alterations the client pays for are in. Shown only
  // when there are any: Total above stays the booking as it was made.
  const adjustedTotal =
    total !== null && row.adjustments.length > 0
      ? total +
        row.adjustments.reduce(
          (sum, adjustment) => sum + adjustment.client_amount_cents,
          0,
        )
      : null
  const calculo = buildCalculo(row)
  const referenceForMessage = {
    loyaltyPrograms,
    airlines,
    accounts,
    agencies,
    people,
  }

  const programLocatorLabel = program
    ? `Localizador ${program.name}`
    : "Localizador"
  const airlineLocatorLabel = airline
    ? `Localizador ${airline.name}`
    : "Localizador da cia"

  // ---- the field blocks -------------------------------------------------------------

  const programAndAirline = (
    <FieldGroup>
      <ComboField
        name="program"
        label="Programa de fidelidade"
        options={programOptions}
        selectedKey={row.loyalty_program_id}
        displayValue={program?.name}
        onPick={(option) => pickProgram(option.key)}
        onFreeText={(typed) => {
          const match = findByCodeOrName(
            loyaltyPrograms.filter((candidate) => !candidate.deleted_at),
            typed,
          )
          if (match) pickProgram(match.id)
          return Boolean(match)
        }}
        onSlash={() => focusField(formRef.current, "airline")}
        tint={lookupTint(programTint)}
        readOnly={lookupLocked}
      />
      <ComboField
        name="airline"
        label="Cia aérea"
        options={airlineOptions}
        selectedKey={row.airline_id}
        displayValue={airline?.name}
        onPick={(option) => patch({ airline_id: option.key })}
        onFreeText={(typed) => {
          const match = findByCodeOrName(airlines, typed)
          if (match) patch({ airline_id: match.id })
          return Boolean(match)
        }}
        tint={lookupTint(airlineTint)}
        readOnly={lookupLocked}
      />
    </FieldGroup>
  )

  const locators = (
    <FieldGroup>
      <TextField
        name="program_locator"
        label={showSecondLocator ? programLocatorLabel : "Localizador"}
        value={row.program_locator}
        uppercase
        onCommit={(value) => patch({ program_locator: value })}
        onSlash={() => {
          setSecondLocatorRequested(true)
          requestAnimationFrame(() =>
            focusField(formRef.current, "airline_locator"),
          )
        }}
        tint={lookupTint(programTint)}
        readOnly={lookupLocked}
      />
      {showSecondLocator ? (
        <TextField
          name="airline_locator"
          label={airlineLocatorLabel}
          value={row.airline_locator}
          uppercase
          onCommit={(value) => patch({ airline_locator: value })}
          tint={lookupTint(airlineTint)}
          readOnly={lookupLocked}
        />
      ) : null}
      {showPurchaseNumber ? (
        <TextField
          name="latam_purchase_number"
          label="Nº da compra"
          value={row.latam_purchase_number}
          onCommit={(value) => patch({ latam_purchase_number: value })}
          tint={lookupTint(airlineTint)}
          readOnly={lookupLocked}
        />
      ) : null}
    </FieldGroup>
  )

  const originField = (
    <ComboField
      name="origin"
      label="Origem"
      options={(typed) => airportOptions(airports, typed)}
      selectedKey={row.origin_iata_code}
      displayValue={row.origin_iata_code ?? ""}
      onPick={(option) => patch({ origin_iata_code: option.key })}
      onFreeText={(typed) => {
        // A three-letter code is taken as typed even while the airport list is loading.
        if (!/^[a-z]{3}$/i.test(typed)) return false
        patch({ origin_iata_code: typed.toUpperCase() })
        return true
      }}
      tint={showLookupView ? lookupTint(null) : itineraryTint}
      readOnly={showLookupView ? lookupLocked : itineraryLocked}
    />
  )

  const accountField = (
    <ComboField
      name="account"
      label="Conta"
      options={accountChoices}
      selectedKey={selectedAccountKey}
      displayValue={accountDisplay}
      busy={Boolean(board.pendingAccountLabels[row.key])}
      onPick={(option) => {
        const choice = accountChoices.find(
          (candidate) => candidate.key === option.key,
        )
        if (choice)
          patch({
            account_id: choice.accountId,
            account_nickname_id: choice.nicknameId,
          })
      }}
      create={{
        label: (typed) => {
          const { nickname } = parseAccountEntry(typed)
          const exists = accountChoices.some(
            (option) =>
              normalizeSearch(option.label) === normalizeSearch(nickname),
          )
          return exists
            ? `Usar a conta "${nickname}"`
            : `Criar conta "${nickname}"`
        },
        hint: (typed) => {
          const { administratorName } = parseAccountEntry(typed)
          return administratorName
            ? `vincula ${administratorName} como administrador`
            : "cria a pessoa, a conta no programa e o apelido"
        },
        disabledReason: row.loyalty_program_id
          ? null
          : "Escolha o programa antes de criar uma conta",
        onCreate: (typed) => void board.resolveAccountEntry(row.key, typed),
      }}
      invalid={accountInvalid}
      hint={
        accountInvalid
          ? "Escolha uma conta deste programa - a emissão não é salva sem ela."
          : null
      }
      readOnly={cancelled}
    />
  )

  const passengerField = (
    <TextField
      name="main_passenger_name"
      label="Passageiro principal"
      value={row.main_passenger_name}
      uppercase
      onCommit={(value) => patch({ main_passenger_name: value || null })}
      tint={itineraryTint}
      readOnly={itineraryLocked}
    />
  )

  // The surname the airline is asked with, apart from the passenger's name: the sync
  // replaces that with the full name, and a refetch would then ask with it. Only in the
  // simplified view, and only where it means something - the autocomplete is on and this
  // airline wants a surname. The full view is about the issuance, not the lookup.
  const showSurname = Boolean(
    autocompleteEnabled && airline?.requires_last_name && autocompletes,
  )
  const surnameField = (
    <TextField
      name="passenger_last_name"
      label="Sobrenome"
      value={row.passenger_last_name}
      uppercase
      onCommit={(value) => patch({ passenger_last_name: value || null })}
      tint={lookupTint(null)}
      readOnly={lookupLocked}
    />
  )

  // Only what this airline is asked with: the origin for GOL and Azul (requires_origin),
  // the surname where it is required.
  const lookupInputs = [
    airline?.requires_origin ? originField : null,
    showSurname ? surnameField : null,
  ].filter(Boolean)

  // ---- the view ---------------------------------------------------------------------

  const lookupFields = (
    <>
      {programAndAirline}
      {locators}
      {program && airline && autocompletes && (
        <>
          {lookupInputs.length > 0 && (
            <FieldGroup>
              {airline.requires_origin ? originField : null}
              {showSurname ? surnameField : null}
            </FieldGroup>
          )}
          {accountField}
        </>
      )}
      {lookingUp && (
        <p className="flex items-center gap-2 text-sm text-sky-600">
          <Loader2 className="h-4 w-4 animate-spin" />
          Buscando a reserva na {airline?.name ?? "cia"}. Os valores já podem
          ser preenchidos.
        </p>
      )}
    </>
  )

  const fullFields = (
    <>
      {programAndAirline}
      {locators}
      <div className="grid gap-4 sm:grid-cols-2 sm:gap-x-6">
        <FieldGroup>
          {originField}
          <ComboField
            name="destination"
            label="Destino"
            options={(typed) => airportOptions(airports, typed)}
            selectedKey={row.destination_iata_code}
            displayValue={row.destination_iata_code ?? ""}
            onPick={(option) => patch({ destination_iata_code: option.key })}
            onFreeText={(typed) => {
              if (!/^[a-z]{3}$/i.test(typed)) return false
              patch({ destination_iata_code: typed.toUpperCase() })
              return true
            }}
            tint={itineraryTint}
            readOnly={itineraryLocked}
          />
        </FieldGroup>
        <DateField
          name="flight_date"
          label="Data do voo"
          value={row.flight_date}
          onCommit={(value) => patch({ flight_date: value })}
          tint={itineraryTint}
          readOnly={itineraryLocked}
        />
        {passengerField}
        {accountField}
        <ComboField
          name="agency"
          label="Agência"
          options={agencies.map((agency) => ({
            key: agency.id,
            label: agency.name,
          }))}
          selectedKey={row.agency_id}
          onPick={(option) => patch({ agency_id: option.key })}
          readOnly={cancelled}
        />
        <ComboField
          name="issuer"
          label="Emissor"
          options={users.map((user) => ({ key: user.id, label: user.name }))}
          selectedKey={row.issuer_id}
          onPick={(option) => patch({ issuer_id: option.key })}
          readOnly={cancelled}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-3 sm:gap-x-6">
        <TextField
          name="observation"
          label="Observação"
          value={row.observation}
          onCommit={(value) => patch({ observation: value || null })}
          className="sm:col-span-2"
          readOnly={cancelled}
        />
        <DateField
          name="issued_on"
          label="Data da emissão"
          value={row.issued_on}
          required
          onCommit={(value) => {
            if (value) patch({ issued_on: value })
          }}
          readOnly={cancelled}
        />
      </div>
      {acceptsRedemptionFee(row, accounts) && (
        <NumberField
          name="redemption_fee"
          label="Taxa de resgate economizada"
          mode="currency"
          value={row.redemption_fee_saved_cents}
          onCommit={(value) => patch({ redemption_fee_saved_cents: value })}
          className="sm:w-1/3"
          readOnly={cancelled}
        />
      )}
    </>
  )

  // The cancel dialog reads its handlers from the grid's context. Only these two are used
  // by it; the rest of the shape is the grid's and is never called from the panel.
  const cancelContext = useMemo<GridContextValue>(
    () => ({
      loyaltyPrograms,
      airlines,
      accounts,
      agencies,
      users,
      people,
      airports,
      onCellClick: () => {},
      onSaveAndMove: () => {},
      onExitEdit: () => {},
      registerActiveCellSave: () => {},
      requestSaveActiveCell: () =>
        (document.activeElement as HTMLElement | null)?.blur(),
      onDeleteRow: () => {},
      onCancelRow: (_index, data) => board.cancelRow(row.key, data),
      onUncancelRow: () => board.uncancelRow(row.key),
      onSyncRowWithTraveos: () => board.syncRow(row.key),
      syncingRowKeys: board.syncingRowKeys,
      resolveAccountEntryForRow: () => {},
      pendingAccountLabels: board.pendingAccountLabels,
    }),
    [
      loyaltyPrograms,
      airlines,
      accounts,
      agencies,
      users,
      people,
      airports,
      board,
      row.key,
    ],
  )

  const [revertOpen, setRevertOpen] = useState(false)

  async function handleSync() {
    toast.info(
      row.traveos_synced_at
        ? "Atualizando os dados do voo. Isso pode levar alguns minutos."
        : "Puxando os dados da cia. Isso pode levar alguns minutos.",
    )
    const result = await board.syncRow(row.key)
    if ("error" in result) toast.error(result.error)
    else toast.success("Dados do voo atualizados.")
  }

  const iconButton = "h-8 w-8 [&_svg]:size-5"

  return (
    <GridProvider value={cancelContext}>
      <div
        ref={formRef}
        data-issuance-form
        className="flex flex-col gap-6 px-4 pt-4 pb-10 sm:px-8 sm:pt-8"
        onKeyDown={(event) => {
          // Keys from a menu or dialog portalled out of the panel still bubble here
          // through React; those close themselves and must not minimize the panel too.
          if (!formRef.current?.contains(event.target as Node)) return
          if (event.key === "Escape" && !event.defaultPrevented) {
            event.preventDefault()
            onMinimize()
          }
        }}
      >
        {/* Sticky on a phone, where the panel is the whole screen and this is the way out. */}
        <div className="sticky top-0 z-20 -mx-4 flex items-center justify-between gap-3 bg-slate-200 px-4 py-2 sm:static sm:mx-0 sm:px-0 sm:py-0 lg:pb-6">
          <button
            type="button"
            onClick={onMinimize}
            title="Minimizar (Esc)"
            className="flex items-center gap-2 rounded-full bg-white px-3 py-1.5 text-sm font-medium text-slate-900 shadow-xs hover:bg-slate-50"
          >
            <ArrowLeft className="h-4 w-4" />
            Minimizar emissão
          </button>
          <div className="flex items-center gap-2">
            <span className="flex h-8 min-w-8 items-center justify-center rounded-full bg-white px-2 text-sm font-medium text-slate-900 tabular-nums">
              {number}
            </span>
            <StatusPill row={row} onChange={(status) => patch({ status })} />
          </div>
        </div>

        <section className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
              <h2
                ref={headingRef}
                tabIndex={-1}
                className="text-xl font-semibold text-slate-500 outline-none sm:text-2xl"
              >
                Dados da emissão
              </h2>
              <label className="flex cursor-pointer items-center gap-2 text-sm sm:text-base">
                <Switch
                  checked={autocompleteEnabled}
                  onCheckedChange={setAutocompleteEnabled}
                  className="data-[state=checked]:bg-green-500"
                />
                <span
                  className={
                    autocompleteEnabled ? "text-green-600" : "text-slate-500"
                  }
                >
                  Autocompletar dados
                </span>
              </label>
            </div>
            <div className="flex items-center gap-0.5">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className={iconButton}
                    title="Copiar a emissão"
                  >
                    <Copy className="text-slate-500" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={() =>
                      void copyEmissaoText(
                        buildEmissaoResumo(row, referenceForMessage),
                        "resumo",
                      )
                    }
                  >
                    <MessageSquareText />
                    Copiar resumo
                    <span className="text-muted-foreground ml-auto text-xs">
                      Ctrl+Alt+C
                    </span>
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => void copyEmissaoText(calculo, "cálculo")}
                  >
                    <Calculator />
                    Copiar cálculo
                    <span className="text-muted-foreground ml-auto text-xs">
                      Ctrl+Alt+X
                    </span>
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>

              {/* Gated on the itinerary having been fetched, not on "complete": a ticket from a
                  hand-ticked row would have no flights and no passengers. */}
              {row.id && row.traveos_synced_at ? (
                <Button
                  asChild
                  variant="ghost"
                  size="icon"
                  className={iconButton}
                  title="Ver bilhete"
                >
                  <Link href={`/dashboard/issuances/${row.id}/ticket`}>
                    <Ticket className="text-violet-600" />
                  </Link>
                </Button>
              ) : (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className={iconButton}
                  disabled
                  title="O bilhete aparece depois que os dados do voo forem puxados"
                >
                  <Ticket className="text-violet-600" />
                </Button>
              )}

              <Button
                type="button"
                variant="ghost"
                size="icon"
                className={iconButton}
                disabled={!row.id || cancelled || syncing}
                title={
                  !row.id
                    ? "Os dados do voo são puxados de novo depois que a emissão for salva"
                    : row.traveos_synced_at
                      ? "Atualizar os dados do voo"
                      : "Puxar os dados do voo"
                }
                onClick={() => void handleSync()}
              >
                {syncing ? (
                  <Loader2 className="animate-spin text-sky-600" />
                ) : (
                  <CloudDownload className="text-slate-500" />
                )}
              </Button>

              {row.id && !cancelled && (
                // The grid's cancel dialog, whose trigger hides until its row is hovered -
                // forced visible and to this row's size here.
                <span className="[&_button]:!h-8 [&_button]:!w-8 [&_button]:!opacity-100 [&_svg]:!size-5 [&_svg]:!text-orange-500">
                  <CancelIssuanceButton r={number - 1} />
                </span>
              )}
              {row.id && cancelled && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className={iconButton}
                  title="Reverter cancelamento"
                  onClick={() => setRevertOpen(true)}
                >
                  <RotateCcw className="text-emerald-600" />
                </Button>
              )}

              <Button
                type="button"
                variant="ghost"
                size="icon"
                className={iconButton}
                title="Excluir emissão (Ctrl+Shift+⌫)"
                onClick={() => onDeleteRequestedChange(true)}
              >
                <Trash2 className="text-red-600" />
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-4">
            {showLookupView ? lookupFields : fullFields}
          </div>

          {lookupViewPossible && (
            <button
              type="button"
              onClick={() => setPreferLookupView((current) => !current)}
              className="flex cursor-pointer items-center gap-1.5 self-end rounded-sm text-sm text-slate-500 underline-offset-4 outline-none hover:text-slate-700 hover:underline focus-visible:text-slate-700 focus-visible:underline"
            >
              {showLookupView
                ? "Visualizar informações completas"
                : "Visualizar informações simplificadas"}
              <ChevronsUpDown className="h-4 w-4" />
            </button>
          )}
        </section>

        <hr className="border-slate-300" />

        <section className="flex flex-col gap-4">
          <h2 className="text-xl font-semibold text-slate-500 sm:text-2xl">
            Valores da emissão
          </h2>
          <FieldGroup>
            <NumberField
              name="points"
              label="Pontos"
              mode="points"
              value={row.points}
              onCommit={(value) => patch({ points: value })}
              readOnly={cancelled}
            />
            <NumberField
              name="value_of_the_thousand"
              label="Milheiro"
              mode="currency"
              value={row.value_of_the_thousand}
              onCommit={(value) => patch({ value_of_the_thousand: value })}
              readOnly={cancelled}
            />
            {/* The one value a cancelled issuance still takes: the refunded taxes differ
                from what was charged often enough to be corrected after the fact. */}
            <NumberField
              name="taxes"
              label="Taxas"
              mode="currency"
              value={row.taxes}
              onCommit={(value) => patch({ taxes: value })}
            />
            <DisplayField
              label="Total"
              value={total !== null ? formatCurrencyBRL(total) : ""}
            />
          </FieldGroup>
          {adjustedTotal !== null && (
            <p className="self-end text-sm font-medium text-slate-700 tabular-nums">
              Total com alterações: {formatCurrencyBRL(adjustedTotal)}
            </p>
          )}
          <div className="flex flex-wrap items-center justify-end gap-x-3 gap-y-1 text-sm text-slate-500">
            {row.redemption_fee_saved_cents !== null && (
              <>
                <CopyableText
                  text={`Taxa de resgate = ${formatCurrencyBRL(row.redemption_fee_saved_cents)}`}
                  copyValue={formatCurrencyBRL(row.redemption_fee_saved_cents)}
                  message="Taxa de resgate copiada"
                />
                <span className="text-slate-400">|</span>
              </>
            )}
            {"text" in calculo ? (
              <CopyableText
                text={calculo.text.replace(/= (.*)$/, "= R$ $1")}
                copyValue={calculo.text}
                message="Cálculo copiado"
              />
            ) : (
              <span>Preencha pontos e milheiro para ver o cálculo</span>
            )}
          </div>
        </section>

        {/* A draft has nothing to alter yet: an alteration is about a ticket already issued. */}
        {row.id && (
          <>
            <hr className="border-slate-300" />
            <AdjustmentsSection board={board} row={row} />
          </>
        )}
      </div>

      <AlertDialog
        open={deleteRequested}
        onOpenChange={onDeleteRequestedChange}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Deletar emissão</AlertDialogTitle>
            <AlertDialogDescription>
              Você realmente deseja deletar essa emissão?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-red-300 text-red-600 hover:bg-red-50 hover:text-red-700">
              Não
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                const result = await board.deleteRow(row.key)
                if ("error" in result) toast.error(result.error)
                else onMinimize()
              }}
            >
              Sim, deletar
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={revertOpen} onOpenChange={setRevertOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Reverter cancelamento</AlertDialogTitle>
            <AlertDialogDescription>
              A emissão voltará a ficar ativa e editável, e o pagamento da venda
              vinculada será restaurado ao estado anterior ao cancelamento.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Voltar</AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                const result = await board.uncancelRow(row.key)
                if ("error" in result) toast.error(result.error)
                else toast.success("Cancelamento revertido.")
              }}
            >
              Reverter
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </GridProvider>
  )
}

function CopyableText({
  text,
  copyValue,
  message,
}: {
  text: string
  copyValue: string
  message: string
}) {
  return (
    <button
      type="button"
      onClick={() => void copyToClipboard(copyValue, message)}
      className="flex items-center gap-1.5 tabular-nums hover:text-slate-700"
      title="Copiar"
    >
      {text}
      <Copy className="h-3.5 w-3.5" />
    </button>
  )
}

// The status of the issuance, and the way to tick it complete by hand. Cancelled is not on
// offer here - cancelling captures the refund, which is what the cancel button is for.
function StatusPill({
  row,
  onChange,
}: {
  row: IssuanceRow
  onChange: (status: IssuanceStatus) => void
}) {
  const pillClass = cn(
    "flex items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium",
    row.status === "complete" && "bg-emerald-100 text-emerald-800",
    row.status === "incomplete" && "bg-red-100 text-red-800",
    row.status === "cancelled" && "bg-slate-300 text-slate-700",
  )
  const dot = (
    <span
      className={cn(
        "h-2.5 w-2.5 rounded-full",
        row.status === "complete" && "bg-emerald-700",
        row.status === "incomplete" && "bg-red-600",
        row.status === "cancelled" && "bg-slate-600",
      )}
    />
  )

  if (row.isDraft || row.status === "cancelled") {
    return (
      <span
        className={pillClass}
        title={row.isDraft ? "Ainda não salva" : undefined}
      >
        {dot}
        {row.isDraft ? "Rascunho" : STATUS_LABELS[row.status]}
      </span>
    )
  }

  // Both states on show, the way a switch reads: the one the issuance is in, coloured, and
  // the other one beside it to click. A single pill with a menu behind it did not look
  // like something that could be changed.
  return (
    <div
      role="radiogroup"
      aria-label="Status da emissão"
      className="flex items-center rounded-full bg-white p-1 shadow-xs"
    >
      {(["incomplete", "complete"] as const).map((status) => {
        const active = row.status === status
        return (
          <button
            key={status}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => {
              if (!active) onChange(status)
            }}
            title={
              active
                ? undefined
                : `Marcar como ${STATUS_LABELS[status].toLowerCase()}`
            }
            className={cn(
              "flex cursor-pointer items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-sky-200",
              active &&
                status === "complete" &&
                "bg-emerald-100 text-emerald-800",
              active && status === "incomplete" && "bg-red-100 text-red-800",
              !active &&
                "text-slate-400 hover:bg-slate-100 hover:text-slate-700",
            )}
          >
            <span
              className={cn(
                "h-2.5 w-2.5 rounded-full",
                status === "complete" ? "bg-emerald-600" : "bg-red-600",
                !active && "opacity-40",
              )}
            />
            {STATUS_LABELS[status]}
          </button>
        )
      })}
    </div>
  )
}
