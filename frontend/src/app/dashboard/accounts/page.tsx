import type { Metadata } from "next"
import Link from "next/link"
import { TriangleAlert } from "lucide-react"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { fetchList } from "@/lib/fetch-list"
import { ListPagination } from "@/components/shared/list-pagination"
import {
  getAccountOptionsSchema,
  listAccountHoldersSchema,
  type AccountHolderPage,
  type AccountOption,
  type AccountHolderRow as AccountHolderRowData,
} from "@/schemas/account"
import { getPeopleOptionsSchema, PersonOption } from "@/schemas/person"
import { getLoyaltyProgramsSchema, LoyaltyProgram } from "@/schemas/loyalty-program"
import { AccountHolderRow } from "@/components/accounts/account-holder-row"
import { CreateAccountForm } from "@/components/accounts/create-account-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"
import { getSessionUser } from "@/lib/session"
import { canAccess } from "@/lib/navigation"
import { ACCOUNT_CATEGORY_OPTIONS } from "@/lib/account-category"

export const metadata: Metadata = {
  title: "Contas",
  description:
    "Os titulares e as contas de programa de fidelidade de onde saem os pontos, com as credenciais de acesso.",
}

const ACCOUNTS_URL_KEYS = [
  "q",
  "order",
  "page",
  "loyalty_program_id",
  "category",
  "complete",
  "pending",
] as const

const PAGE_LIMIT = 20

// The to-do lists the page reports, keyed by the value of ?pending= - each maps to the API's
// <key>_only filter and <key>_total count. "Accounts without a holder" is gone: the database
// refuses them since migration 75.
const PENDING = {
  duplicate_programs: {
    filter: "Programa repetido",
    one: "1 titular tem duas contas no mesmo programa - mescle a que sobra na outra.",
    many: (count: number) =>
      `${count} titulares têm duas contas no mesmo programa - mescle a que sobra na outra.`,
  },
  admin_conflicts: {
    filter: "Administradores diferentes",
    one: "1 titular tem contas com administradores diferentes - escolha um na seção Titular.",
    many: (count: number) =>
      `${count} titulares têm contas com administradores diferentes - escolha um na seção Titular.`,
  },
} as const

type PendingKey = keyof typeof PENDING
type PendingTotals = Record<PendingKey, number>

// Read by hand rather than through fetchPage: besides the rows and the total, the page
// needs the pending totals for the banner. A failed request reads as an empty page.
async function fetchHolderPage(
  url: string,
  authHeader: Record<string, string>,
): Promise<{ items: AccountHolderRowData[]; total: number; pending: PendingTotals }> {
  const empty = {
    items: [],
    total: 0,
    pending: { duplicate_programs: 0, admin_conflicts: 0 },
  }
  try {
    const res = await fetch(url, { headers: authHeader, cache: "no-store" })
    const parsed = listAccountHoldersSchema.response.safeParse(await res.json())
    if (!res.ok || !parsed.success || !parsed.data.data) return empty
    const data: AccountHolderPage = parsed.data.data
    return {
      items: data.rows ?? [],
      total: data.total,
      pending: {
        duplicate_programs: data.duplicate_programs_total,
        admin_conflicts: data.admin_conflicts_total,
      },
    }
  } catch {
    return empty
  }
}

export default async function AccountsPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof ACCOUNTS_URL_KEYS)[number], string>>>
}) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const raw = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/accounts"),
    ACCOUNTS_URL_KEYS,
  )
  const params = resolveListParams(raw)
  const loyaltyProgramId = raw.loyalty_program_id ?? ""
  const category = raw.category ?? ""
  const complete = raw.complete ?? ""
  const pending = raw.pending ?? ""

  // Merging moves issuance history and then deletes records for good, so the panel only
  // offers it to admins - the backend refuses it for anyone else regardless.
  const sessionUser = await getSessionUser()
  const canMerge = canAccess(sessionUser?.role, ["admin"])

  const accountsQuery = new URLSearchParams(
    listQueryString(params, {
      loyalty_program_id: loyaltyProgramId,
      category,
      complete,
      ...(pending in PENDING ? { [`${pending}_only`]: "true" } : {}),
    }),
  )
  accountsQuery.set("page", String(params.page))
  accountsQuery.set("limit", String(PAGE_LIMIT))

  // One wave instead of four sequential round trips: none of these lists depends
  // on another, and the page cannot paint until the slowest of them is in.
  const [rowsPage, peopleResult, loyaltyProgramsResult, orphanAccountsResult] = await Promise.all([
    // /accounts/holders pages over holders, not accounts: a holder with three programs
    // is one row, so the page size is what the operator actually sees.
    fetchHolderPage(
      `${env.BACKEND_URL}/api/v1/accounts/holders?${accountsQuery.toString()}`,
      authHeader,
    ),
    // view=picker: esta lista só alimenta comboboxes, e sem ela a página
    // carregava as catorze colunas de cada pessoa cadastrada.
    fetchList<PersonOption>(
      `${env.BACKEND_URL}/api/v1/people?view=picker`,
      getPeopleOptionsSchema.response,
      authHeader,
    ),
    fetchList<LoyaltyProgram>(
      `${env.BACKEND_URL}/api/v1/loyalty-programs`,
      getLoyaltyProgramsSchema.response,
      authHeader,
    ),
    // Accounts nobody holds are rows of their own, and the merge picker offers them next to
    // the people. Unfiltered by the current search for the same reason the people are: the
    // duplicate is usually the row the search does not show. Archived ones included, since
    // they block the NOT NULL on holder_id too and merging is how they get a holder.
    canMerge
      ? fetchList<AccountOption>(
          `${env.BACKEND_URL}/api/v1/accounts?view=picker&include_archived=true`,
          getAccountOptionsSchema.response,
          authHeader,
        )
      : Promise.resolve(null),
  ])

  // A failed list renders as an empty one: every consumer below is a row list or a
  // combobox, and both read the same either way.
  const rows: AccountHolderRowData[] = rowsPage.items
  const pendingLines = (Object.keys(PENDING) as PendingKey[])
    .map((key) => ({ key, count: rowsPage.pending[key], labels: PENDING[key] }))
    .filter((line) => line.count > 0)
  const people: PersonOption[] = peopleResult ?? []
  const loyaltyPrograms: LoyaltyProgram[] = loyaltyProgramsResult ?? []
  const orphanAccounts: AccountOption[] = (orphanAccountsResult ?? []).filter(
    (account) => !account.holder_id,
  )

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Contas</h1>

        <CreateAccountForm people={people} loyaltyPrograms={loyaltyPrograms} />
        <ListFilterBar
          // The CPF is stored encrypted, so it is matched whole rather than by fragment -
          // the placeholder says so, because "CPF" alone would promise a partial search.
          searchPlaceholder="Buscar por nome, CPF completo, login, programa ou apelido"
          order={params.order}
        >
          <ListFilterSelect
            name="loyalty_program_id"
            label="Programa"
            allLabel="Todos"
            value={loyaltyProgramId}
            className="w-52"
            options={loyaltyPrograms.map((program) => ({
              value: program.id,
              label: program.name,
            }))}
          />
          <ListFilterSelect
            name="category"
            label="Categoria"
            allLabel="Todas"
            value={category}
            className="w-44"
            options={ACCOUNT_CATEGORY_OPTIONS}
          />
          <ListFilterSelect
            name="complete"
            label="Cadastro"
            allLabel="Todos"
            value={complete}
            options={[
              { value: "true", label: "Completos" },
              { value: "false", label: "Incompletos" },
            ]}
          />
          <ListFilterSelect
            name="pending"
            label="Pendência"
            allLabel="Todas as linhas"
            value={pending}
            className="w-52"
            options={Object.entries(PENDING).map(([value, labels]) => ({
              value,
              label: labels.filter,
            }))}
          />
        </ListFilterBar>

        {/* What still needs an operator: rows the database cannot yet be made to refuse
            (a second account in a program, two administrators for one holder, an account
            with no holder). Unfiltered counts, so the banner reads the same whatever the
            current search shows. */}
        {pendingLines.length > 0 && (
          <div className="flex flex-col gap-1.5 rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900">
            {pendingLines.map(({ key, count, labels }) => (
              <div key={key} className="flex flex-wrap items-center justify-between gap-2">
                <span className="flex items-center gap-2">
                  <TriangleAlert className="h-4 w-4 shrink-0" />
                  {count === 1 ? labels.one : labels.many(count)}
                </span>
                {pending !== key && (
                  <Link
                    href={`/dashboard/accounts?pending=${key}`}
                    className="font-medium underline underline-offset-2"
                  >
                    Ver só essas
                  </Link>
                )}
              </div>
            ))}
          </div>
        )}

        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {rows.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || loyaltyProgramId || category || complete || pending
                ? "Nenhuma conta encontrada para os filtros selecionados."
                : "Nenhuma conta cadastrada."}
            </p>
          ) : (
            rows.map((row) => (
              <AccountHolderRow
                key={row.holder?.id ?? row.accounts[0]?.id}
                row={row}
                people={people}
                loyaltyPrograms={loyaltyPrograms}
                orphanAccounts={orphanAccounts}
                canMerge={canMerge}
              />
            ))
          )}
        </div>

        <ListPagination
          total={rowsPage.total}
          page={params.page}
          limit={PAGE_LIMIT}
          noun={["conta", "contas"]}
        />
      </div>
    </div>
  )
}
