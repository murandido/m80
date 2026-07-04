import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getLoyaltyProgramsSchema, LoyaltyProgram } from "@/schemas/loyalty-program"
import { getAirlinesSchema, Airline } from "@/schemas/airline"
import { LoyaltyProgramsAirlinesToggle } from "@/components/layout/loyalty-programs-airlines-toggle"
import { LoyaltyProgramCard } from "@/components/loyalty-programs/loyalty-program-card"
import { CreateLoyaltyProgramForm } from "@/components/loyalty-programs/create-loyalty-program-form"

export default async function LoyaltyProgramsPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let loyaltyPrograms: LoyaltyProgram[] = []
  let airlines: Airline[] = []

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
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {loyaltyPrograms.map((loyaltyProgram) => (
            <LoyaltyProgramCard
              key={loyaltyProgram.id}
              loyaltyProgram={loyaltyProgram}
              airlines={airlines}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
