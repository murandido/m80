import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { Airport, listAirportsSchema } from "@/schemas/airport"
import { CreateAirportForm } from "@/components/airports/create-airport-form"
import { AirportCard } from "@/components/airports/airport-card"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListPagination } from "@/components/shared/list-pagination"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"
import {
  AIRPORT_RELEVANCE,
  airportOrderOptions,
  resolveAirportOrder,
} from "@/lib/list-params"

export const metadata: Metadata = {
  title: "Aeroportos",
  description:
    "A base de códigos IATA que preenche a origem e o destino das emissões.",
}

const PAGE_LIMIT = 20

const AIRPORTS_URL_KEYS = ["q", "order", "page"] as const

export default async function AirportsPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof AIRPORTS_URL_KEYS)[number], string>>>
}) {
  await requireAdmin()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const { q, order, page: pageParam } = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/airports"),
    AIRPORTS_URL_KEYS,
  )
  const query = q?.trim() ?? ""
  const airportOrder = resolveAirportOrder(order, Boolean(q?.trim()))
  const page = Math.max(1, Number(pageParam) || 1)

  let airports: Airport[] = []
  let total = 0

  try {
    const params = new URLSearchParams({
      page: String(page),
      limit: String(PAGE_LIMIT),
    })
    if (query) params.set("q", query)
    // Relevance is the absence of an order, not a value the backend knows.
    if (airportOrder !== AIRPORT_RELEVANCE) params.set("order", airportOrder)

    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/airports/manage?${params.toString()}`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )

    const json = await res.json()
    const parsed = listAirportsSchema.response.safeParse(json)

    if (parsed.success && parsed.data.data) {
      airports = parsed.data.data.airports ?? []
      total = parsed.data.data.total
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Tabela de Aeroportos
        </h1>
        <CreateAirportForm />
        <ListFilterBar
          searchPlaceholder="Buscar por código IATA, nome, cidade, estado ou país"
          order={airportOrder}
          orderOptions={airportOrderOptions(Boolean(query))}
        />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {airports.length === 0 ? (
            <p className="text-sm text-slate-500">
              {query
                ? `Nenhum aeroporto encontrado para "${query}".`
                : "Nenhum aeroporto cadastrado."}
            </p>
          ) : (
            airports.map((airport) => (
              <AirportCard key={airport.id} airport={airport} />
            ))
          )}
        </div>
        <ListPagination
          total={total}
          page={page}
          limit={PAGE_LIMIT}
          noun={["aeroporto", "aeroportos"]}
        />
      </div>
    </div>
  )
}
