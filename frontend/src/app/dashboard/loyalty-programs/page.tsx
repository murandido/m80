import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { requireDev } from "@/lib/session"
import { env } from "@/lib/env"
import { getLoyaltyProgramsSchema, LoyaltyProgram } from "@/schemas/loyalty-program"
import { getAirlinesSchema, Airline } from "@/schemas/airline"
import { LoyaltyProgramsAirlinesToggle } from "@/components/layout/loyalty-programs-airlines-toggle"
import { LoyaltyProgramCard } from "@/components/loyalty-programs/loyalty-program-card"
import { CreateLoyaltyProgramForm } from "@/components/loyalty-programs/create-loyalty-program-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Programas de Fidelidade",
  description:
    "Os programas de pontos, a companhia de cada um e a cor com que aparecem na grid.",
}

const PROGRAMS_URL_KEYS = ["q", "order"] as const

export default async function LoyaltyProgramsPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof PROGRAMS_URL_KEYS)[number], string>>>
}) {
  await requireDev()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const params = resolveListParams(
    stickySearchParams(
      await searchParams,
      readStickyFilters(cookieStore, "/dashboard/loyalty-programs"),
      PROGRAMS_URL_KEYS,
    ),
  )

  let loyaltyPrograms: LoyaltyProgram[] = []
  let airlines: Airline[] = []

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/loyalty-programs?${listQueryString(params)}`,
      { headers: authHeader, cache: "no-store" },
    )
    const json = await res.json()

    const parsed = getLoyaltyProgramsSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      loyaltyPrograms = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/airlines`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getAirlinesSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      airlines = parsed.data.data
    }
  } catch {
    // silently fail — combobox just comes up empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Programas de Fidelidade e CIAs Aéreas
        </h1>

        <LoyaltyProgramsAirlinesToggle active="loyalty-programs" />

        <CreateLoyaltyProgramForm airlines={airlines} />
        <ListFilterBar
          searchPlaceholder="Buscar por nome ou código"
          order={params.order}
        />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {loyaltyPrograms.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q
                ? `Nenhum programa encontrado para "${params.q}".`
                : "Nenhum programa cadastrado."}
            </p>
          ) : (
            loyaltyPrograms.map((loyaltyProgram) => (
              <LoyaltyProgramCard
                key={loyaltyProgram.id}
                loyaltyProgram={loyaltyProgram}
                airlines={airlines}
              />
            ))
          )}
        </div>
      </div>
    </div>
  )
}
