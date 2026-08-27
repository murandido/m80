import type { Metadata } from "next"
import { cookies } from "next/headers"
import { Separator } from "@/components/ui/separator"
import { requireDev } from "@/lib/session"
import { env } from "@/lib/env"
import {
  getPromoEnrollmentsSchema,
  getPromoRecipesSchema,
  type PromoEnrollment,
  type PromoRecipe,
} from "@/schemas/promo"
import { getLoyaltyProgramsSchema, type LoyaltyProgram } from "@/schemas/loyalty-program"
import { CreateRecipeForm } from "@/components/promotions/create-recipe-form"
import { RecipeCard } from "@/components/promotions/recipe-card"
import { EnrollmentHistory } from "@/components/promotions/enrollment-history"
import { ListFilterBar } from "@/components/shared/list-filter-bar"
import { ListFilterSelect } from "@/components/shared/list-filter-select"
import { listQueryString, resolveListParams } from "@/lib/list-params"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"

export const metadata: Metadata = {
  title: "Automação de Promoções",
  description:
    "As receitas que ensinam o bot a se cadastrar nas promoções de transferência, e o histórico do que já foi cadastrado.",
}

const PROMOTIONS_URL_KEYS = ["q", "order", "active"] as const

export default async function PromotionsPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<(typeof PROMOTIONS_URL_KEYS)[number], string>>>
}) {
  await requireDev()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }

  const raw = stickySearchParams(
    await searchParams,
    readStickyFilters(cookieStore, "/dashboard/promotions"),
    PROMOTIONS_URL_KEYS,
  )
  const params = resolveListParams(raw)
  const active = raw.active ?? ""

  let recipes: PromoRecipe[] = []
  let programs: LoyaltyProgram[] = []
  let enrollments: PromoEnrollment[] = []

  try {
    const [recipesRes, programsRes, enrollmentsRes] = await Promise.all([
      fetch(
        `${env.BACKEND_URL}/api/v1/promo-recipes?${listQueryString(params, { active })}`,
        { headers: authHeader, cache: "no-store" },
      ),
      fetch(`${env.BACKEND_URL}/api/v1/loyalty-programs`, {
        headers: authHeader,
        cache: "no-store",
      }),
      fetch(`${env.BACKEND_URL}/api/v1/promo-enrollments?limit=20`, {
        headers: authHeader,
        cache: "no-store",
      }),
    ])

    const parsedRecipes = getPromoRecipesSchema.response.safeParse(await recipesRes.json())
    if (parsedRecipes.success && parsedRecipes.data.data) {
      recipes = parsedRecipes.data.data
    }

    const parsedPrograms = getLoyaltyProgramsSchema.response.safeParse(
      await programsRes.json(),
    )
    if (parsedPrograms.success && parsedPrograms.data.data) {
      programs = parsedPrograms.data.data
    }

    const parsedEnrollments = getPromoEnrollmentsSchema.response.safeParse(
      await enrollmentsRes.json(),
    )
    if (parsedEnrollments.success && parsedEnrollments.data.data?.enrollments) {
      enrollments = parsedEnrollments.data.data.enrollments
    }
  } catch {
    // silently fail — lists stay empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        <div className="flex flex-col gap-1">
          <h1 className="text-lg font-semibold text-slate-800">Automação de Promoções</h1>
          <p className="text-sm text-slate-500">
            Uma receita por site. Quando alguém manda o link de uma promoção num grupo
            com o cadastro ligado, o bot abre a página e inscreve os CPFs daquele
            programa.
          </p>
        </div>

        <CreateRecipeForm programs={programs} />

        <ListFilterBar searchPlaceholder="Buscar por nome ou domínio" order={params.order}>
          <ListFilterSelect
            name="active"
            label="Situação"
            allLabel="Todas"
            value={active}
            options={[
              { value: "true", label: "Ativas" },
              { value: "false", label: "Inativas" },
            ]}
          />
        </ListFilterBar>

        <Separator className="my-2" />

        <div className="flex flex-col gap-2">
          {recipes.length === 0 ? (
            <p className="text-sm text-slate-500">
              {params.q || active
                ? "Nenhuma receita encontrada para os filtros selecionados."
                : "Nenhuma receita cadastrada."}
            </p>
          ) : (
            recipes.map((recipe) => (
              <RecipeCard key={recipe.id} recipe={recipe} programs={programs} />
            ))
          )}
        </div>

        <Separator className="my-2" />

        <div className="flex flex-col gap-3">
          <h2 className="text-base font-semibold text-slate-800">
            Últimos cadastros
          </h2>
          <EnrollmentHistory enrollments={enrollments} />
        </div>
      </div>
    </div>
  )
}
