import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { Airport, listAirportsSchema } from "@/schemas/airport"
import { CreateAirportForm } from "@/components/airports/create-airport-form"
import { AirportCard } from "@/components/airports/airport-card"
import { AirportSearchBar } from "@/components/airports/airport-search-bar"
import { AirportPagination } from "@/components/airports/airport-pagination"

const PAGE_LIMIT = 20

export default async function AirportsPage({
  searchParams,
}: {
  searchParams: Promise<{ q?: string; page?: string }>
}) {
  await requireAdmin()

  const { q, page: pageParam } = await searchParams
  const query = q?.trim() ?? ""
  const page = Math.max(1, Number(pageParam) || 1)

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  let airports: Airport[] = []
  let total = 0

  try {
    const params = new URLSearchParams({
      page: String(page),
      limit: String(PAGE_LIMIT),
    })
    if (query) params.set("q", query)

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
        <AirportSearchBar />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {airports.length === 0 ? (
            <p className="text-sm text-slate-500">Nenhum aeroporto encontrado.</p>
          ) : (
            airports.map((airport) => (
              <AirportCard key={airport.id} airport={airport} />
            ))
          )}
        </div>
        <AirportPagination total={total} page={page} limit={PAGE_LIMIT} />
      </div>
    </div>
  )
}
