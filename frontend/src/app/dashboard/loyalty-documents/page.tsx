import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { requireAdmin } from "@/lib/session"
import { env } from "@/lib/env"
import {
  listLoyaltyDocumentsSchema,
  type LoyaltyDocument,
  type LoyaltyDocumentPage,
} from "@/schemas/promo"
import {
  getLoyaltyProgramsSchema,
  type LoyaltyProgram,
} from "@/schemas/loyalty-program"
import { CreateDocumentForm } from "@/components/loyalty-documents/create-document-form"
import { DocumentCard } from "@/components/loyalty-documents/document-card"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { fetchList, fetchPage } from "@/lib/fetch-list"
import { ListPagination } from "@/components/shared/list-pagination"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "CPFs das Promoções",
  description:
    "Os CPFs da agência que o bot inscreve nas promoções de transferência de pontos.",
}

const DOCUMENTS_URL_KEYS = ["q", "order", "page", "active"] as const

const PAGE_LIMIT = 20

export default async function LoyaltyDocumentsPage({
  searchParams,
}: {
  searchParams: Promise<
    Partial<Record<(typeof DOCUMENTS_URL_KEYS)[number], string>>
  >
}) {
  await requireAdmin()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const raw = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/loyalty-documents"),
    DOCUMENTS_URL_KEYS,
  )
  const params = resolveListParams(raw)
  const active = raw.active ?? ""

  const query = new URLSearchParams(listQueryString(params, { active }))
  query.set("page", String(params.page))
  query.set("limit", String(PAGE_LIMIT))

  const [page, programs] = await Promise.all([
    // /loyalty-documents/manage rather than /loyalty-documents: the plain route stays
    // unpaginated because the create action reads it back whole.
    fetchPage<LoyaltyDocument, LoyaltyDocumentPage>(
      `${env.BACKEND_URL}/api/v1/loyalty-documents/manage?${query.toString()}`,
      listLoyaltyDocumentsSchema.response,
      (data) => data.documents,
      authHeader,
    ),
    fetchList<LoyaltyProgram>(
      `${env.BACKEND_URL}/api/v1/loyalty-programs`,
      getLoyaltyProgramsSchema.response,
      authHeader,
    ),
  ])

  const documents = page.items
  const programOptions = programs ?? []

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <div className="flex flex-col gap-1">
          <h1 className="text-lg font-semibold text-slate-800">
            CPFs das Promoções
          </h1>
          <p className="text-sm text-slate-500">
            Quem entra nas promoções de transferência. O número fica
            criptografado no banco; aqui ele aparece porque a equipe precisa
            dele.
          </p>
        </div>

        <CreateDocumentForm programs={programOptions} />

        <ListFilterBar searchPlaceholder="Buscar por nome" order={params.order}>
          <ListFilterSelect
            name="active"
            label="Situação"
            allLabel="Todos"
            value={active}
            options={[
              { value: "true", label: "Ativos" },
              { value: "false", label: "Inativos" },
            ]}
          />
        </ListFilterBar>

        <Separator className="my-2" />

        <div className="flex flex-col gap-2">
          {documents.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || active
                ? "Nenhum CPF encontrado para os filtros selecionados."
                : "Nenhum CPF cadastrado."}
            </p>
          ) : (
            documents.map((document) => (
              <DocumentCard
                key={document.id}
                document={document}
                programs={programOptions}
              />
            ))
          )}
        </div>

        <ListPagination
          total={page.total}
          page={params.page}
          limit={PAGE_LIMIT}
          noun={["CPF", "CPFs"]}
        />
      </div>
    </div>
  )
}
