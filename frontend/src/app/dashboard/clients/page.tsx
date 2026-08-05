import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { getClientsSchema, Client } from "@/schemas/client"
import { ClientCard } from "@/components/clients/client-card"
import { CreateClientForm } from "@/components/clients/create-client-form"

export const metadata: Metadata = {
  title: "Clientes",
  description:
    "Quem compra as passagens: o nome e o telefone usados para cobrar cada emissão.",
}

export default async function ClientsPage() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  let clients: Client[] = []

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/clients`, {
      headers: authHeader,
    })
    const json = await res.json()

    const parsed = getClientsSchema.response.safeParse(json)
    if (parsed.success && parsed.data.data) {
      clients = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Clientes</h1>

        <CreateClientForm />
        <Separator className="my-2" />
        <div className="flex flex-col gap-2">
          {clients.map((client) => (
            <ClientCard key={client.id} client={client} />
          ))}
        </div>
      </div>
    </div>
  )
}
