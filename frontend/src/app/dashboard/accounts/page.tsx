import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { fetchList, fetchPage } from "@/lib/fetch-list"
import { ListPagination } from "@/components/shared/list-pagination"
import {
  getAccountOptionsSchema,
  listAccountsSchema,
  type Account,
  type AccountOption,
  type AccountPage,
} from "@/schemas/account"
import { getPeopleOptionsSchema, PersonOption } from "@/schemas/person"
import { getLoyaltyProgramsSchema, LoyaltyProgram } from "@/schemas/loyalty-program"
import { ContasPessoasToggle } from "@/components/layout/contas-pessoas-toggle"
import { AccountCard } from "@/components/accounts/account-card"
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
    "As contas de programa de fidelidade de onde saem os pontos, com as credenciais de acesso.",
}

const ACCOUNTS_URL_KEYS = ["q", "order", "page", "loyalty_program_id", "category", "complete"] as const

const PAGE_LIMIT = 20

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

  // Merging moves issuance history and then deletes a record for good, so the panel only
  // offers it to admins - the backend refuses it for anyone else regardless.
  const sessionUser = await getSessionUser()
  const canMerge = canAccess(sessionUser?.role, ["admin"])

  const accountsQuery = new URLSearchParams(
    listQueryString(params, {
      loyalty_program_id: loyaltyProgramId,
      category,
      complete,
    }),
  )
  accountsQuery.set("page", String(params.page))
  accountsQuery.set("limit", String(PAGE_LIMIT))

  // One wave instead of four sequential round trips: none of these lists depends
  // on another, and the page cannot paint until the slowest of them is in.
  const [allAccountsResult, accountsPage, peopleResult, loyaltyProgramsResult] =
    await Promise.all([
      // Unfiltered, because the duplicate the operator is after is usually exactly the
      // one the current search does not show - the two are registered differently. And
      // archived ones included, because archiving was the only way to get a duplicate off
      // the screen before merging existed: those are the ones still holding issuances.
      canMerge
        ? fetchList<AccountOption>(
            `${env.BACKEND_URL}/api/v1/accounts?include_archived=true&view=picker`,
            getAccountOptionsSchema.response,
            authHeader,
          )
        : Promise.resolve(null),
      // /accounts/manage rather than /accounts: the plain route stays unpaginated
      // because the issuances grid, the sales screens and the importer all read it
      // whole for their pickers.
      fetchPage<Account, AccountPage>(
        `${env.BACKEND_URL}/api/v1/accounts/manage?${accountsQuery.toString()}`,
        listAccountsSchema.response,
        (data) => data.accounts,
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
    ])

  // A failed list renders as an empty one: every consumer below is a card grid or a
  // combobox, and both read the same either way.
  const allAccounts: AccountOption[] = allAccountsResult ?? []
  const accounts: Account[] = accountsPage.items
  const people: PersonOption[] = peopleResult ?? []
  const loyaltyPrograms: LoyaltyProgram[] = loyaltyProgramsResult ?? []

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Contas e Proprietários
        </h1>

        <ContasPessoasToggle active="contas" />

        <CreateAccountForm people={people} loyaltyPrograms={loyaltyPrograms} />
        <ListFilterBar
          searchPlaceholder="Buscar por login, titular, administrador, programa ou apelido"
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
        </ListFilterBar>
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {accounts.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || loyaltyProgramId || category || complete
                ? "Nenhuma conta encontrada para os filtros selecionados."
                : "Nenhuma conta cadastrada."}
            </p>
          ) : (
            accounts.map((account) => (
              <AccountCard
                key={account.id}
                account={account}
                people={people}
                loyaltyPrograms={loyaltyPrograms}
                allAccounts={allAccounts}
                canMerge={canMerge}
              />
            ))
          )}
        </div>

        <ListPagination
          total={accountsPage.total}
          page={params.page}
          limit={PAGE_LIMIT}
          noun={["conta", "contas"]}
        />
      </div>
    </div>
  )
}
