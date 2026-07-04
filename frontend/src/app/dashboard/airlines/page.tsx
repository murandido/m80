import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getAirlinesSchema, Airline } from "@/schemas/airline"
import { LoyaltyProgramsAirlinesToggle } from "@/components/layout/loyalty-programs-airlines-toggle"
import { AirlineCard } from "@/components/airlines/airline-card"
import { CreateAirlineForm } from "@/components/airlines/create-airline-form"

export default async function AirlinesPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let airlines: Airline[] = []

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
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Programas de Fidelidade e CIAs Aéreas
        </h1>

        <LoyaltyProgramsAirlinesToggle active="airlines" />

        <CreateAirlineForm />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {airlines.map((airline) => (
            <AirlineCard key={airline.id} airline={airline} />
          ))}
        </div>
      </div>
    </div>
  )
}
