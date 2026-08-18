import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getPeopleSchema, listPeopleSchema, Person } from "@/schemas/person"
import { getSessionUser } from "@/lib/session"
import { canAccess } from "@/lib/navigation"
import { ContasPessoasToggle } from "@/components/layout/contas-pessoas-toggle"
import { PersonCard } from "@/components/people/person-card"
import { CreatePersonForm } from "@/components/people/create-person-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { ListPagination } from "@/components/shared/list-pagination"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Proprietários",
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

  // Merging rewrites who owns an account and then deletes a record for good, so the panel
  // only offers it to admins - the backend refuses it for anyone else regardless.
  const sessionUser = await getSessionUser()
  const canMerge = canAccess(sessionUser?.role, ["admin"])
  let allPeople: Person[] = []

  if (canMerge) {
    try {
      // The unpaginated list, because the duplicate the operator is after is usually
      // exactly the one the current page or search does not show.
      const res = await fetch(`${env.BACKEND_URL}/api/v1/people`, {
        headers: authHeader,
        cache: "no-store",
      })
      const json = await res.json()

      const parsed = getPeopleSchema.response.safeParse(json)
      if (parsed.success && parsed.data.data) {
        allPeople = parsed.data.data
      }
    } catch {
      // silently fail — the merge picker just comes up empty
    }
  }

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
          Contas e Proprietários
        </h1>

        <ContasPessoasToggle active="pessoas" />

        <CreatePersonForm />
        <ListFilterBar
          // The CPF is stored encrypted, so it is matched whole rather than by fragment -
          // the placeholder says so, because "CPF" alone would promise a partial search that
          // silently returns nothing.
          searchPlaceholder="Buscar por nome, e-mail, telefone, cidade, apelido ou CPF completo"
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
                ? "Nenhum proprietário encontrado para os filtros selecionados."
                : "Nenhum proprietário cadastrado."}
            </p>
          ) : (
            people.map((person) => (
              <PersonCard
                key={person.id}
                person={person}
                people={allPeople}
                canMerge={canMerge}
              />
            ))
          )}
        </div>
        <ListPagination
          total={total}
          page={params.page}
          limit={PAGE_LIMIT}
          noun={["proprietário", "proprietários"]}
        />
      </div>
    </div>
  )
}
