import type { Metadata } from "next"
import { getSessionUser, verifySession } from "@/lib/session"
import { userRoleLabels } from "@/schemas/user"
import { ProfileForm } from "@/components/users/profile-form"

export const metadata: Metadata = {
  title: "Meu perfil",
  description: "Seus dados de acesso ao painel: nome, telefone e senha.",
}

export default async function ProfilePage() {
  await verifySession()
  // Served from the render-wide cache the layout already filled - no second /auth/me.
  const user = await getSessionUser()

  return (
    <div className="p-6 sm:p-8">
      <div className="flex max-w-xl flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Meu perfil</h1>

        {user ? (
          <>
            <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 rounded-md border border-slate-200 p-4 text-sm">
              <dt className="text-slate-500">Nome</dt>
              <dd className="text-slate-800">{user.name}</dd>
              <dt className="text-slate-500">Telefone</dt>
              <dd className="text-slate-800">{user.phone}</dd>
              <dt className="text-slate-500">Perfil</dt>
              <dd className="text-slate-800">{userRoleLabels[user.role]}</dd>
            </dl>
            <ProfileForm user={user} />
          </>
        ) : (
          <p className="text-sm text-slate-500">
            Não foi possível carregar seus dados agora. Tente recarregar a
            página.
          </p>
        )}
      </div>
    </div>
  )
}
