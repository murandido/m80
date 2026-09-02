import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { fetchPage } from "@/lib/fetch-list"
import { listClientsSchema, type Client, type ClientPage } from "@/schemas/client"
import { ClientCard } from "@/components/clients/client-card"
import { CreateClientForm } from "@/components/clients/create-client-form"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListPagination } from "@/components/shared/list-pagination"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Clientes",
  description:
    "Quem compra as passagens: o nome e o telefone usados para cobrar cada emissão.",
}

const CLIENTS_URL_KEYS = ["q", "order", "page"] as const

const PAGE_LIMIT = 20

export default async function ClientsPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof CLIENTS_URL_KEYS)[number], string>>>
}) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const raw = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/clients"),
    CLIENTS_URL_KEYS,
  )
  const params = resolveListParams(raw)

  const query = new URLSearchParams(listQueryString(params))
  query.set("page", String(params.page))
  query.set("limit", String(PAGE_LIMIT))

  // /clients/manage rather than /clients: the plain route stays unpaginated because
  // the sales screen's client picker reads it whole.
  const page = await fetchPage<Client, ClientPage>(
    `${env.BACKEND_URL}/api/v1/clients/manage?${query.toString()}`,
    listClientsSchema.response,
    (data) => data.clients,
    authHeader,
  )

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Clientes</h1>

        <CreateClientForm />

        {/* A client has only a name and a phone, and "who is this number" is half of
            why anyone opens this screen - so the search covers both. */}
        <ListFilterBar
          searchPlaceholder="Buscar por nome ou telefone"
          order={params.order}
        />

        <Separator className="my-2" />

        <div className="flex flex-col gap-2">
          {page.items.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q
                ? "Nenhum cliente encontrado para a busca."
                : "Nenhum cliente cadastrado."}
            </p>
          ) : (
            page.items.map((client) => (
              <ClientCard key={client.id} client={client} />
            ))
          )}
        </div>

        <ListPagination
          total={page.total}
          page={params.page}
          limit={PAGE_LIMIT}
          noun={["cliente", "clientes"]}
        />
      </div>
    </div>
  )
}
