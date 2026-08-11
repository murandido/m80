import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getAccountsSchema, Account } from "@/schemas/account"
import { getPeopleSchema, Person } from "@/schemas/person"
import { getLoyaltyProgramsSchema, LoyaltyProgram } from "@/schemas/loyalty-program"
import { ContasPessoasToggle } from "@/components/layout/contas-pessoas-toggle"
import { AccountCard } from "@/components/accounts/account-card"
import { CreateAccountForm } from "@/components/accounts/create-account-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Contas",
  description:
    "As contas de programa de fidelidade de onde saem os pontos, com as credenciais de acesso.",
}

const ACCOUNTS_URL_KEYS = ["q", "order", "loyalty_program_id", "complete"] as const

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
  const complete = raw.complete ?? ""

  let accounts: Account[] = []
  let people: Person[] = []
  let loyaltyPrograms: LoyaltyProgram[] = []

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/accounts?${listQueryString(params, {
        loyalty_program_id: loyaltyProgramId,
        complete,
      })}`,
      { headers: authHeader, cache: "no-store" },
    )
    const json = await res.json()

    const parsed = getAccountsSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      accounts = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getPeopleSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      people = parsed.data.data
    }
  } catch {
    // silently fail — combobox/title lookups just come up empty
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/loyalty-programs`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getLoyaltyProgramsSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      loyaltyPrograms = parsed.data.data
    }
  } catch {
    // silently fail — combobox just comes up empty
  }

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
              {params.q || loyaltyProgramId || complete
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
              />
            ))
          )}
        </div>
      </div>
    </div>
  )
}
