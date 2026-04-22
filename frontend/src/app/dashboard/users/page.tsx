import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getUsersSchema, User } from "@/schemas/user"
import { UserCard } from "@/components/users/user-card"
import { CreateUserForm } from "@/components/users/create-user-form"

export default async function UsersPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  let users: User[] = []

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/users`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const json = await res.json()
    const parsed = getUsersSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      users = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div>
      <div className="mx-12 my-16 sm:mx-24 sm:my-32 lg:mr-12">
        <div>
          <div className="mx-2 flex flex-col gap-2 sm:gap-6">
            <h1 className="text-lg font-semibold text-slate-800">
              Tabela de Usuários
            </h1>
            <CreateUserForm />
            <Separator className="my-2" />
            <div className="flex flex-col gap-2">
              {users.map((user) => (
                <UserCard key={user.id} user={user} />
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
