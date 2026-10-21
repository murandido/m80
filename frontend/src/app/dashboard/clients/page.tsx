import type { Metadata } from "next"
import { cookies } from "next/headers"
import Link from "next/link"
import { TriangleAlert } from "lucide-react"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { fetchList, fetchPage } from "@/lib/fetch-list"
import { getSessionUser } from "@/lib/session"
import { Agency, getAgenciesSchema } from "@/schemas/agency"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
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

const CLIENTS_URL_KEYS = ["q", "order", "page", "pending_agency"] as const

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
  const pendingAgency = raw.pending_agency === "true" ? "true" : ""

  const query = new URLSearchParams(listQueryString(params))
  query.set("page", String(params.page))
  query.set("limit", String(PAGE_LIMIT))
  if (pendingAgency) query.set("pending_agency", "true")

  // Only a dev ever sees a client without an agency - the API keeps them out of everyone
  // else's list - so only a dev's page asks how many are left.
  const pendingQuery = new URLSearchParams({ pending_agency: "true", page: "1", limit: "1" })

  const sessionUser = await getSessionUser()
  const viewerIsDev = sessionUser?.role === "dev"

  // /clients/manage rather than /clients: the plain route stays unpaginated because
  // the sales screen's client picker reads it whole.
  const [page, agencies, pending] = await Promise.all([
    fetchPage<Client, ClientPage>(
      `${env.BACKEND_URL}/api/v1/clients/manage?${query.toString()}`,
      listClientsSchema.response,
      (data) => data.clients,
      authHeader,
    ),
    fetchList<Agency>(`${env.BACKEND_URL}/api/v1/agencies`, getAgenciesSchema.response, authHeader),
    viewerIsDev
      ? fetchPage<Client, ClientPage>(
          `${env.BACKEND_URL}/api/v1/clients/manage?${pendingQuery.toString()}`,
          listClientsSchema.response,
          (data) => data.clients,
          authHeader,
        )
      : null,
  ])
  const pendingTotal = pending?.total ?? 0

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Clientes</h1>

        <CreateClientForm agencies={agencies ?? []} />

        {/* A client has only a name and a phone, and "who is this number" is half of
            why anyone opens this screen - so the search covers both, plus the locators
            of the client's issuances for when the booking code is all anyone has. */}
        <ListFilterBar
          searchPlaceholder="Buscar por nome, telefone ou localizador"
          order={params.order}
        >
          {viewerIsDev && (
            <ListFilterSelect
              name="pending_agency"
              label="Agência"
              allLabel="Todas"
              value={pendingAgency}
              options={[{ value: "true", label: "Sem agência" }]}
            />
          )}
        </ListFilterBar>

        {/* The to-do list for making clients.agency_id NOT NULL: the clients migration
            000072 could not place because they bought through more than one agency. */}
        {pendingTotal > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900">
            <span className="flex items-center gap-2">
              <TriangleAlert className="h-4 w-4 shrink-0" />
              {pendingTotal === 1
                ? "1 cliente ainda está sem agência"
                : `${pendingTotal} clientes ainda estão sem agência`}
              . Eles compraram por mais de uma agência e só aparecem para desenvolvedores até
              alguém escolher a agência de cada um.
            </span>
            {pendingAgency !== "true" && (
              <Link
                href="/dashboard/clients?pending_agency=true"
                className="font-medium underline underline-offset-2"
              >
                Ver só esses
              </Link>
            )}
          </div>
        )}

        <Separator className="my-2" />

        <div className="flex flex-col gap-2">
          {page.items.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || pendingAgency
                ? "Nenhum cliente encontrado para a busca."
                : "Nenhum cliente cadastrado."}
            </p>
          ) : (
            page.items.map((client) => (
              <ClientCard key={client.id} client={client} agencies={agencies ?? []} />
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
