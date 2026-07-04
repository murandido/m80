import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getPeopleSchema, Person } from "@/schemas/person"
import { ContasPessoasToggle } from "@/components/layout/contas-pessoas-toggle"
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

        <ContasPessoasToggle active="pessoas" />

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
