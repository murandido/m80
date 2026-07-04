import { cookies } from "next/headers"
import { Landmark, Users } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { cn } from "@/lib/utils"
import { env } from "@/lib/env"
import { getPeopleSchema, Person } from "@/schemas/person"
import { PersonCard } from "@/components/people/person-card"
import { CreatePersonForm } from "@/components/people/create-person-form"

export default async function PeoplePage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let people: Person[] = []

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getPeopleSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      people = parsed.data.data
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

        {/* Contas/Pessoas toggle — Contas is disabled until its CRUD exists on the backend */}
        <div className="flex gap-2">
          <Button
            type="button"
            disabled
            title="Em breve"
            className="flex-1 cursor-not-allowed bg-slate-100 text-slate-400 hover:bg-slate-100"
          >
            <div className="flex w-full items-center justify-center gap-2 p-1 text-sm">
              <Landmark className="h-4 w-4" />
              Contas
            </div>
          </Button>
          <Button
            type="button"
            className={cn(
              "flex-1 bg-slate-800 hover:bg-slate-900",
            )}
          >
            <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
              <Users className="h-4 w-4" />
              Pessoas
            </div>
          </Button>
        </div>

        <CreatePersonForm />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {people.map((person) => (
            <PersonCard key={person.id} person={person} />
          ))}
        </div>
      </div>
    </div>
  )
}
