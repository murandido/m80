import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { getUsersSchema, getMeSchema, User } from "@/schemas/user"
import { UserCard } from "@/components/users/user-card"
import { CreateUserForm } from "@/components/users/create-user-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Usuários do M80",
  description:
    "Quem tem acesso ao painel, com qual perfil e com qual telefone entra.",
}

const USERS_URL_KEYS = ["q", "order", "role"] as const

export default async function UsersPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof USERS_URL_KEYS)[number], string>>>
}) {
  await requireAdmin()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const raw = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/users"),
    USERS_URL_KEYS,
  )
  const params = resolveListParams(raw)
  const role = raw.role ?? ""

  let users: User[] = []
  let currentUserId: string | null = null

  try {
    const [usersRes, meRes] = await Promise.all([
      fetch(
        `${env.BACKEND_URL}/api/v1/users?${listQueryString(params, { role })}`,
        { headers: authHeader, cache: "no-store" },
      ),
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
          Usuários do M80
        </h1>
        <CreateUserForm />
        <ListFilterBar
          searchPlaceholder="Buscar por nome ou telefone"
          order={params.order}
        >
          <ListFilterSelect
            name="role"
            label="Perfil"
            allLabel="Todos"
            value={role}
            options={[
              { value: "admin", label: "Administrador" },
              { value: "user", label: "Usuário" },
              { value: "dev", label: "Desenvolvedor" },
            ]}
          />
        </ListFilterBar>
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {users.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || role
                ? "Nenhum usuário encontrado para os filtros selecionados."
                : "Nenhum usuário cadastrado."}
            </p>
          ) : (
            users.map((user) => (
              <UserCard
                key={user.id}
                user={user}
                isCurrentUser={user.id === currentUserId}
              />
            ))
          )}
        </div>
      </div>
    </div>
  )
}
