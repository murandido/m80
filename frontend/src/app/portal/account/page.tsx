import type { Metadata } from "next"
import { PortalPasswordForm } from "@/components/portal/portal-password-form"

export const metadata: Metadata = {
  title: "Minha senha · M80",
  description: "Troque a senha que você usa para entrar no M80.",
}

export default function PortalAccountSettingsPage() {
  return (
    <div className="mx-auto flex w-full max-w-md flex-col gap-4 px-4 py-6">
      <div>
        <h1 className="text-lg font-semibold text-slate-800">Minha senha</h1>
        <p className="mt-1 text-sm text-slate-500">
          É a senha que você usa para entrar no M80. As senhas da Azul e da Smiles ficam na
          página de cada conta.
        </p>
      </div>
      <PortalPasswordForm />
    </div>
  )
}
