import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { requireAdmin } from "@/lib/session"
import { env } from "@/lib/env"
import {
  getLoyaltyDocumentsSchema,
  type LoyaltyDocument,
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
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "CPFs das Promoções",
  description:
    "Os CPFs da agência que o bot inscreve nas promoções de transferência de pontos.",
}

const DOCUMENTS_URL_KEYS = ["q", "order", "active"] as const

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

  let documents: LoyaltyDocument[] = []
  let programs: LoyaltyProgram[] = []

  try {
    const [documentsRes, programsRes] = await Promise.all([
      fetch(
        `${env.BACKEND_URL}/api/v1/loyalty-documents?${listQueryString(params, { active })}`,
        { headers: authHeader, cache: "no-store" },
      ),
      fetch(`${env.BACKEND_URL}/api/v1/loyalty-programs`, {
        headers: authHeader,
        cache: "no-store",
      }),
    ])

    const parsedDocuments = getLoyaltyDocumentsSchema.response.safeParse(
      await documentsRes.json(),
    )
    if (parsedDocuments.success && parsedDocuments.data.data) {
      documents = parsedDocuments.data.data
    }

    const parsedPrograms = getLoyaltyProgramsSchema.response.safeParse(
      await programsRes.json(),
    )
    if (parsedPrograms.success && parsedPrograms.data.data) {
      programs = parsedPrograms.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

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

        <CreateDocumentForm programs={programs} />

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
                programs={programs}
              />
            ))
          )}
        </div>
      </div>
    </div>
  )
}
