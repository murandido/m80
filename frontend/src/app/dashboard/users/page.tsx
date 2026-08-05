import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { getUsersSchema, getMeSchema, User } from "@/schemas/user"
import { UserCard } from "@/components/users/user-card"
import { CreateUserForm } from "@/components/users/create-user-form"

export const metadata: Metadata = {
  title: "Usuários",
  description:
    "Quem tem acesso ao painel, com qual perfil e com qual telefone entra.",
}

export default async function UsersPage() {
  await requireAdmin()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let users: User[] = []
  let currentUserId: string | null = null

  try {
    const [usersRes, meRes] = await Promise.all([
      fetch(`${env.BACKEND_URL}/api/v1/users`, { headers: authHeader }),
      fetch(`${env.BACKEND_URL}/api/v1/auth/me`, { headers: authHeader }),
    ])

    const [usersJson, meJson] = await Promise.all([usersRes.json(), meRes.json()])

    const parsedUsers = getUsersSchema.response.safeParse(usersJson)
    if (parsedUsers.success && parsedUsers.data.data) {
      users = parsedUsers.data.data
    }

    const parsedMe = getMeSchema.response.safeParse(meJson)
    if (parsedMe.success && parsedMe.data.data) {
      currentUserId = parsedMe.data.data.id
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Tabela de Usuários
        </h1>
        <CreateUserForm />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {users.map((user) => (
            <UserCard
              key={user.id}
              user={user}
              isCurrentUser={user.id === currentUserId}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
