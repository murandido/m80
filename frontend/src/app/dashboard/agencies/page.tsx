import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { requireDev } from "@/lib/session"
import { env } from "@/lib/env"
import { getAgenciesSchema, Agency } from "@/schemas/agency"
import { AgencyCard } from "@/components/agencies/agency-card"
import { CreateAgencyForm } from "@/components/agencies/create-agency-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Agências",
  description:
    "As agências que emitem pelo painel e qual delas vem marcada por padrão nos filtros.",
}

const AGENCIES_URL_KEYS = ["q", "order"] as const

export default async function AgenciesPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof AGENCIES_URL_KEYS)[number], string>>>
}) {
  // Dev only: devs decide which agencies each account sees, so creating one - or moving the
  // main agency - is theirs too. The API refuses the writes to anyone else.
  await requireDev()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const params = resolveListParams(
    stickySearchParams(
      await searchParams,
      readStickyFilters(cookieStore, "/dashboard/agencies"),
      AGENCIES_URL_KEYS,
    ),
  )

  let agencies: Agency[] = []

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/agencies?${listQueryString(params)}`,
      { headers: authHeader, cache: "no-store" },
    )
    const json = await res.json()

    const parsed = getAgenciesSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      agencies = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Agências</h1>

        <CreateAgencyForm />
        <ListFilterBar
          searchPlaceholder="Buscar por nome ou apelido"
          order={params.order}
        />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {agencies.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q
                ? `Nenhuma agência encontrada para "${params.q}".`
                : "Nenhuma agência cadastrada."}
            </p>
          ) : (
            agencies.map((agency) => (
              <AgencyCard key={agency.id} agency={agency} />
            ))
          )}
        </div>
      </div>
    </div>
  )
}
