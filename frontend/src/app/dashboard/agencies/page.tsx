import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getAgenciesSchema, Agency } from "@/schemas/agency"
import { AgencyCard } from "@/components/agencies/agency-card"
import { CreateAgencyForm } from "@/components/agencies/create-agency-form"

export const metadata: Metadata = {
  title: "Agências",
  description:
    "As agências que emitem pelo painel e qual delas vem marcada por padrão nos filtros.",
}

export default async function AgenciesPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let agencies: Agency[] = []

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/agencies`, {
      headers: authHeader,
    })
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
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {agencies.map((agency) => (
            <AgencyCard key={agency.id} agency={agency} />
          ))}
        </div>
      </div>
    </div>
  )
}
