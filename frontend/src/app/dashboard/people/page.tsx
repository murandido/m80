import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { listPeopleSchema, Person } from "@/schemas/person"
import { ContasPessoasToggle } from "@/components/layout/contas-pessoas-toggle"
import { PersonCard } from "@/components/people/person-card"
import { CreatePersonForm } from "@/components/people/create-person-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { ListPagination } from "@/components/shared/list-pagination"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Pessoas",
  description:
    "Passageiros e titulares das emissões, com os apelidos que o bot reconhece no WhatsApp.",
}

const PEOPLE_URL_KEYS = ["q", "order", "page", "complete"] as const

const PAGE_LIMIT = 20

export default async function PeoplePage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof PEOPLE_URL_KEYS)[number], string>>>
}) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const raw = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/people"),
    PEOPLE_URL_KEYS,
  )
  const params = resolveListParams(raw)
  const complete = raw.complete ?? ""

  let people: Person[] = []
  let total = 0

  try {
    // /people/manage rather than /people: the plain route stays unpaginated
    // because the account, sales and issuance pickers all read it whole.
    const query = new URLSearchParams(listQueryString(params, { complete }))
    query.set("page", String(params.page))
    query.set("limit", String(PAGE_LIMIT))

    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/people/manage?${query.toString()}`,
      { headers: authHeader, cache: "no-store" },
    )
    const json = await res.json()

    const parsed = listPeopleSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      people = parsed.data.data.people ?? []
      total = parsed.data.data.total
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Contas e Pessoas
        </h1>

        <ContasPessoasToggle active="pessoas" />

        <CreatePersonForm />
        <ListFilterBar
          searchPlaceholder="Buscar por nome, CPF, e-mail, telefone, cidade ou apelido"
          order={params.order}
        >
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
          {people.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || complete
                ? "Nenhuma pessoa encontrada para os filtros selecionados."
                : "Nenhuma pessoa cadastrada."}
            </p>
          ) : (
            people.map((person) => <PersonCard key={person.id} person={person} />)
          )}
        </div>
        <ListPagination
          total={total}
          page={params.page}
          limit={PAGE_LIMIT}
          noun={["pessoa", "pessoas"]}
        />
      </div>
    </div>
  )
}
