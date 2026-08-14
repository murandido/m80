"use client"

import { useCallback, useRef, useState } from "react"
import {
  AlertTriangle,
  CheckCircle2,
  FileSpreadsheet,
  Upload,
} from "lucide-react"
import { toast } from "sonner"
import { createAccountForIssuanceAction } from "@/actions/account"
import {
  commitLegacyImportAction,
  previewLegacyImportAction,
} from "@/actions/legacy-import"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import {
  LegacyImportMappings,
  accountChoiceKey,
  splitAccountChoiceKey,
} from "@/components/legacy-import/legacy-import-mappings"
import {
  LegacyImportPreviewTable,
  isSelectable,
  rowKey,
} from "@/components/legacy-import/legacy-import-preview-table"
import { cn } from "@/lib/utils"
import type { Account } from "@/schemas/account"
import type { Agency } from "@/schemas/agency"
import type { Person } from "@/schemas/person"
import type { User } from "@/schemas/user"
import type {
  LegacyImportBlock,
  LegacyImportPendingAccount,
  LegacyImportCommitResult,
  LegacyImportDayPreview,
  LegacyImportDaySummary,
  LegacyImportMappings as Mappings,
  LegacyImportRowSelection,
} from "@/schemas/legacy-import"

interface LegacyImportScreenProps {
  accounts: Account[]
  people: Person[]
  users: User[]
  agencies: Agency[]
}

// The temporary screen that brings a day of the hand-kept spreadsheet into the panel.
// Three steps on one page - file, day, preview - and nothing is written until the last
// button. Delete with the rest of the importer; see backend/internal/legacyimport.
export function LegacyImportScreen({
  accounts,
  people,
  users,
  agencies,
}: LegacyImportScreenProps) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [days, setDays] = useState<LegacyImportDaySummary[]>([])
  const [selectedDate, setSelectedDate] = useState("")
  const [preview, setPreview] = useState<LegacyImportDayPreview | null>(null)

  const [accountChoices, setAccountChoices] = useState<Record<string, string>>(
    {},
  )
  const [issuerChoices, setIssuerChoices] = useState<Record<string, string>>({})
  const [agencyId, setAgencyId] = useState("")
  // Per-row agency overrides, keyed by rowKey. Absent means "follow the day's".
  const [rowAgencies, setRowAgencies] = useState<Record<string, string>>({})
  // Accounts created from inside the mapping combobox, folded into the list the page
  // was rendered with so the new one is selectable without a reload.
  const [createdAccounts, setCreatedAccounts] = useState<Account[]>([])

  // The exception, not the selection: every row the server calls "ok" is in unless the
  // operator took it out, so re-resolving after a mapping never undoes their choices.
  const [deselected, setDeselected] = useState<Set<string>>(new Set())

  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<LegacyImportCommitResult | null>(null)

  // Built fresh on every call so the preview always reflects the controls as they are
  // now - the server does the resolving, and this is the whole of what it is told.
  const buildMappings = useCallback(
    (overrides?: Partial<Mappings>): Mappings => ({
      accounts: Object.entries(accountChoices)
        .filter(([, accountId]) => accountId !== "")
        .map(([key, accountId]) => {
          const [programCode, label] = splitAccountChoiceKey(key)
          return {
            program_code: programCode,
            account_label: label,
            account_id: accountId,
          }
        }),
      issuers: Object.entries(issuerChoices)
        .filter(([, userId]) => userId !== "")
        .map(([issuerName, userId]) => ({
          issuer_name: issuerName,
          user_id: userId,
        })),
      agency_id: agencyId,
      row_agencies: Object.entries(rowAgencies).map(([key, id]) => {
        const [blockIndex, number] = key.split(":")
        return {
          block_index: Number(blockIndex),
          number: Number(number),
          agency_id: id,
        }
      }),
      ...overrides,
    }),
    [accountChoices, issuerChoices, agencyId, rowAgencies],
  )

  const loadPreview = useCallback(
    async (chosenFile: File, date: string, mappings: Mappings) => {
      setBusy(true)
      setError("")

      const body = new FormData()
      body.set("file", chosenFile)
      if (date) {
        body.set("day", date)
        body.set("mappings", JSON.stringify(mappings))
      }

      const response = await previewLegacyImportAction(body)
      setBusy(false)

      if ("error" in response) {
        setError(response.error)
        return
      }

      setDays(response.preview.days)
      setPreview(response.preview.preview ?? null)
      if (response.preview.preview && !agencyId) {
        // Seeded from the server's answer, which is the default agency when the
        // operator has not chosen one.
        setAgencyId(response.preview.preview.agency_id)
      }
    },
    [agencyId],
  )

  async function handleFile(chosen: File | null) {
    setFile(chosen)
    setDays([])
    setPreview(null)
    setSelectedDate("")
    setDeselected(new Set())
    setResult(null)
    setAccountChoices({})
    setIssuerChoices({})
    setRowAgencies({})
    if (chosen) await loadPreview(chosen, "", buildMappings())
  }

  async function handleDay(date: string) {
    if (!file) return
    setSelectedDate(date)
    setDeselected(new Set())
    setResult(null)
    setRowAgencies({})
    await loadPreview(file, date, buildMappings())
  }

  // Every mapping change re-asks the server, because the resolution rules live there and
  // guessing at them here is how the preview would start disagreeing with the import.
  async function reresolve(mappings: Mappings) {
    if (!file || !selectedDate) return
    await loadPreview(file, selectedDate, mappings)
  }

  async function handleAccountChoice(
    programCode: string,
    label: string,
    accountId: string,
  ) {
    const key = accountChoiceKey(programCode, label)
    const next = { ...accountChoices, [key]: accountId }
    setAccountChoices(next)

    const mappings = buildMappings()
    mappings.accounts = Object.entries(next)
      .filter(([, id]) => id !== "")
      .map(([entryKey, id]) => {
        const [code, label] = splitAccountChoiceKey(entryKey)
        return { program_code: code, account_label: label, account_id: id }
      })
    await reresolve(mappings)
  }

  async function handleIssuerChoice(issuerName: string, userId: string) {
    const next = { ...issuerChoices, [issuerName]: userId }
    setIssuerChoices(next)

    const mappings = buildMappings()
    mappings.issuers = Object.entries(next)
      .filter(([, id]) => id !== "")
      .map(([name, id]) => ({ issuer_name: name, user_id: id }))
    await reresolve(mappings)
  }

  // No round trip: the agency is the operator's own choice, not something the server
  // resolves, so re-uploading the workbook to be told it back would only add latency.
  // The commit revalidates the ids anyway.
  function handleAgencyChoice(nextAgencyId: string) {
    setAgencyId(nextAgencyId)
  }

  function handleRowAgency(key: string, nextAgencyId: string) {
    setRowAgencies((current) => ({ ...current, [key]: nextAgencyId }))
  }

  // Registering an account from inside the mapping combobox, the same trio the
  // issuances grid creates from a typed name: person, account, alias. The alias is the
  // spreadsheet's own word for it, so the day resolves the moment it exists.
  async function handleCreateAccount(
    pending: LegacyImportPendingAccount,
    name: string,
  ) {
    setBusy(true)
    const created = await createAccountForIssuanceAction(
      name,
      pending.loyalty_program_id,
    )
    setBusy(false)

    if ("error" in created) {
      // The action's own message when it has one. The fallback covers the union
      // widening a server action's inferred return type goes through - the grid's
      // caller does the same.
      setError(created.error ?? "Não consegui criar a conta.")
      return
    }

    setCreatedAccounts((current) => [...current, created.account])
    toast.success(`Conta "${name}" criada.`)
    await handleAccountChoice(
      pending.program_code,
      pending.account_label,
      created.account.id,
    )
  }

  function toggleRow(key: string) {
    setDeselected((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  function toggleBlock(block: LegacyImportBlock, select: boolean) {
    setDeselected((current) => {
      const next = new Set(current)
      for (const row of block.rows ?? []) {
        if (!isSelectable(row)) continue
        const key = rowKey(block.index, row.number)
        if (select) next.delete(key)
        else next.add(key)
      }
      return next
    })
  }

  const blocks = preview?.blocks ?? []
  const selectedRows: LegacyImportRowSelection[] = blocks.flatMap((block) =>
    (block.rows ?? [])
      .filter(
        (row) =>
          isSelectable(row) && !deselected.has(rowKey(block.index, row.number)),
      )
      .map((row) => ({ block_index: block.index, number: row.number })),
  )
  const totalRows = blocks.reduce(
    (sum, block) => sum + (block.rows ?? []).length,
    0,
  )
  const alreadyImported = blocks.reduce(
    (sum, block) =>
      sum +
      (block.rows ?? []).filter((row) => row.verdict === "already_imported")
        .length,
    0,
  )

  async function handleImport() {
    if (!file || !selectedDate || selectedRows.length === 0) return

    setBusy(true)
    setError("")

    const body = new FormData()
    body.set("file", file)
    body.set("day", selectedDate)
    body.set("mappings", JSON.stringify(buildMappings()))
    body.set("rows", JSON.stringify(selectedRows))

    const response = await commitLegacyImportAction(body)
    setBusy(false)

    if ("error" in response) {
      setError(response.error)
      return
    }

    setResult(response.result)
    toast.success(
      `${response.result.imported} emiss${response.result.imported === 1 ? "ão" : "ões"} importada${
        response.result.imported === 1 ? "" : "s"
      }.`,
    )
    // Re-read the day so the rows that just landed come back as "already_imported" -
    // the screen then shows the state of the world rather than what it did last.
    await loadPreview(file, selectedDate, buildMappings())
  }

  return (
    <div className="space-y-6">
      <section className="space-y-2">
        <h2 className="text-sm font-medium text-slate-800">1. Planilha</h2>
        <div className="flex flex-wrap items-center gap-3">
          <input
            ref={fileRef}
            type="file"
            accept=".xlsx"
            className="hidden"
            onChange={(event) =>
              void handleFile(event.target.files?.[0] ?? null)
            }
          />
          <Button
            type="button"
            variant="outline"
            onClick={() => fileRef.current?.click()}
          >
            <Upload className="h-4 w-4" />
            Escolher arquivo
          </Button>
          {file && (
            <span className="text-muted-foreground flex items-center gap-1.5 text-sm">
              <FileSpreadsheet className="h-4 w-4" />
              {file.name}
            </span>
          )}
          {busy && <Spinner className="h-4 w-4" />}
        </div>
      </section>

      {error && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>Não deu certo</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {days.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-sm font-medium text-slate-800">2. Dia</h2>
          <div className="flex flex-wrap gap-2">
            {days.map((day) => (
              <Button
                key={day.date}
                type="button"
                variant={day.date === selectedDate ? "default" : "outline"}
                size="sm"
                disabled={busy || day.row_count === 0}
                onClick={() => void handleDay(day.date)}
                className={cn("tabular-nums")}
              >
                {day.sheet_name}
                <span className="ml-1 text-xs opacity-70">
                  ({day.row_count})
                </span>
              </Button>
            ))}
          </div>
        </section>
      )}

      {preview && (
        <>
          <section className="space-y-2">
            <h2 className="text-sm font-medium text-slate-800">
              3. Conferir e mapear
            </h2>
            <LegacyImportMappings
              pendingAccounts={preview.pending_accounts ?? []}
              pendingIssuers={preview.pending_issuers ?? []}
              accounts={[...accounts, ...createdAccounts]}
              people={people}
              users={users}
              agencies={agencies}
              accountChoices={accountChoices}
              issuerChoices={issuerChoices}
              agencyId={agencyId || preview.agency_id}
              onAccountChoice={(programCode, label, accountId) =>
                void handleAccountChoice(programCode, label, accountId)
              }
              onIssuerChoice={(issuerName, userId) =>
                void handleIssuerChoice(issuerName, userId)
              }
              onAgencyChoice={handleAgencyChoice}
              onCreateAccount={(pending, name) =>
                void handleCreateAccount(pending, name)
              }
              disabled={busy}
            />
          </section>

          <LegacyImportPreviewTable
            blocks={blocks}
            deselected={deselected}
            onToggleRow={toggleRow}
            onToggleBlock={toggleBlock}
            agencies={agencies}
            rowAgencies={rowAgencies}
            defaultAgencyId={agencyId || preview.agency_id}
            onRowAgency={handleRowAgency}
            disabled={busy}
          />

          {result && (
            <Alert>
              <CheckCircle2 className="h-4 w-4" />
              <AlertTitle>Importação concluída</AlertTitle>
              <AlertDescription>
                <span>
                  {result.imported} emissão(ões) gravada(s)
                  {result.registered_aliases > 0 &&
                    `, ${result.registered_aliases} apelido(s) de conta registrado(s)`}
                  {(result.skipped ?? []).length > 0 &&
                    `, ${(result.skipped ?? []).length} linha(s) pulada(s)`}
                  .
                </span>
                {(result.skipped ?? []).map((skipped) => (
                  <span
                    key={`${skipped.block_index}:${skipped.number}`}
                    className="block text-xs"
                  >
                    Linha {skipped.number} ({skipped.locator}): {skipped.reason}
                  </span>
                ))}
              </AlertDescription>
            </Alert>
          )}

          {/* The count is the last thing read before the button, so it says what the
              button will do rather than leaving it to be counted by eye. */}
          <div className="bg-background/95 sticky bottom-0 flex flex-wrap items-center gap-3 border-t py-3 backdrop-blur">
            <span className="text-sm">
              <strong className="tabular-nums">{selectedRows.length}</strong> de{" "}
              {totalRows} linha{totalRows === 1 ? "" : "s"}{" "}
              {selectedRows.length === 1 ? "será" : "serão"} importada
              {selectedRows.length === 1 ? "" : "s"}
              {alreadyImported > 0 &&
                `; ${alreadyImported} já ${alreadyImported === 1 ? "está" : "estão"} no sistema`}
              .
            </span>
            <Button
              type="button"
              className="ml-auto"
              disabled={busy || selectedRows.length === 0}
              onClick={() => void handleImport()}
            >
              {busy && <Spinner className="h-4 w-4" />}
              Importar {selectedRows.length} linha
              {selectedRows.length === 1 ? "" : "s"}
            </Button>
          </div>
        </>
      )}
    </div>
  )
}
