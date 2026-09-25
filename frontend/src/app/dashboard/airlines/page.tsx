import type { Metadata } from "next"
import { cookies } from "next/headers"
import Link from "next/link"
import { Activity } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { requireDev } from "@/lib/session"
import { env } from "@/lib/env"
import { getAirlinesSchema, Airline } from "@/schemas/airline"
import { LoyaltyProgramsAirlinesToggle } from "@/components/layout/loyalty-programs-airlines-toggle"
import { AirlineCard } from "@/components/airlines/airline-card"
import { CreateAirlineForm } from "@/components/airlines/create-airline-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "CIAs Aéreas",
  description:
    "As companhias que operam os voos e o código que a busca automática usa para encontrá-los.",
}

const AIRLINES_URL_KEYS = ["q", "order", "traveos_supported"] as const

export default async function AirlinesPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof AIRLINES_URL_KEYS)[number], string>>>
}) {
  await requireDev()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const raw = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/airlines"),
    AIRLINES_URL_KEYS,
  )
  const params = resolveListParams(raw)
  const traveosSupported = raw.traveos_supported ?? ""

  let airlines: Airline[] = []

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/airlines?${listQueryString(params, {
        traveos_supported: traveosSupported,
      })}`,
      { headers: authHeader, cache: "no-store" },
    )
    const json = await res.json()

    const parsed = getAirlinesSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      airlines = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Programas de Fidelidade e CIAs Aéreas
        </h1>

        <LoyaltyProgramsAirlinesToggle active="airlines" />

        <div className="flex justify-end">
          <Button asChild size="sm" variant="outline">
            <Link href="/dashboard/airlines/direct-lookups">
              <Activity className="h-4 w-4" />
              Conexões diretas
            </Link>
          </Button>
        </div>

        <CreateAirlineForm />
        <ListFilterBar
          searchPlaceholder="Buscar por nome ou código"
          order={params.order}
        >
          <ListFilterSelect
            name="traveos_supported"
            label="Traveos"
            allLabel="Todas"
            value={traveosSupported}
            options={[
              { value: "true", label: "Com suporte" },
              { value: "false", label: "Sem suporte" },
            ]}
          />
        </ListFilterBar>
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {airlines.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || traveosSupported
                ? "Nenhuma companhia encontrada para os filtros selecionados."
                : "Nenhuma companhia cadastrada."}
            </p>
          ) : (
            airlines.map((airline) => (
              <AirlineCard key={airline.id} airline={airline} />
            ))
          )}
        </div>
      </div>
    </div>
  )
}
