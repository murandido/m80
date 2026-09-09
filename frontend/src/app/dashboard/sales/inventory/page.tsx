import { Suspense } from "react"
import type { Metadata } from "next"
import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { fetchList } from "@/lib/fetch-list"
import { formatYmd } from "@/lib/date-range"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { LiveRefresh } from "@/components/layout/live-refresh"
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { getAgenciesSchema, type Agency } from "@/schemas/agency"
import { getAccountOptionsSchema, type AccountOption } from "@/schemas/account"
import {
  getInventoryClosingSchema,
  getPointBatchesSchema,
  getPointBatchOriginsSchema,
  type InventoryClosing,
  type PointBatch,
  type PointBatchOrigin,
} from "@/schemas/inventory"
import { InventoryToolbar } from "@/components/inventory/inventory-toolbar"
import { InventoryBatchesPanel } from "@/components/inventory/inventory-batches-panel"
import { InventoryClosingPanel } from "@/components/inventory/inventory-closing-panel"
import { StatementPanel } from "@/components/inventory/statement-panel"

export const metadata: Metadata = {
  title: "Estoque de pontos",
}

interface PageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}

function single(value: string | string[] | undefined): string {
  return Array.isArray(value) ? (value[0] ?? "") : (value ?? "")
}

// Reads the closing report, which is a single object rather than a list - fetchList would
// unwrap it as an array.
async function fetchClosing(
  url: string,
  authHeader: Record<string, string>,
): Promise<InventoryClosing | null> {
  try {
    const res = await fetch(url, { headers: authHeader, cache: "no-store" })
    const parsed = getInventoryClosingSchema.response.safeParse(await res.json())
    if (!res.ok || !parsed.success) return null
    return parsed.data.data ?? null
  } catch {
    return null
  }
}

export default async function InventoryPage({ searchParams }: PageProps) {
  await requireAdmin()

  const params = await searchParams
  // A plain from/to, the way the issuances grid carries it - no preset token to resolve.
  // Defaults to the current month so the screen opens on something rather than on nothing.
  const today = new Date()
  const period = {
    date_from:
      single(params.date_from) ||
      formatYmd(new Date(today.getFullYear(), today.getMonth(), 1)),
    date_to: single(params.date_to) || formatYmd(today),
  }

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  const [accounts, agencies, origins] = await Promise.all([
    fetchList<AccountOption>(`${api}/accounts?view=picker`, getAccountOptionsSchema.response, authHeader),
    fetchList<Agency>(`${api}/agencies`, getAgenciesSchema.response, authHeader),
    fetchList<PointBatchOrigin>(
      `${api}/point-batch-origins?active=true`,
      getPointBatchOriginsSchema.response,
      authHeader,
    ),
  ])

  // No account chosen yet means nothing to compute: the report is per account, and asking
  // for every account at once would recompute the whole panel to show a picker.
  const accountIds = single(params.account_ids)
    .split(",")
    .map((id) => id.trim())
    .filter(Boolean)

  const [batches, closing] = accountIds.length
    ? await Promise.all([
        fetchList<PointBatch>(
          `${api}/point-batches?account_id=${encodeURIComponent(accountIds[0])}`,
          getPointBatchesSchema.response,
          authHeader,
        ),
        fetchClosing(
          `${api}/inventory-closing?account_ids=${encodeURIComponent(accountIds.join(","))}` +
            `&from=${period.date_from}&to=${period.date_to}`,
          authHeader,
        ),
      ])
    : [null, null]

  return (
    <div className="flex flex-col gap-6 p-4 md:p-6">
      <LiveRefresh />

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Estoque de pontos</h1>
          <p className="text-muted-foreground text-sm">
            Lotes comprados, custo por milheiro e acerto entre agências.
          </p>
        </div>
        <Button asChild variant="outline">
          <Link href="/dashboard/sales/suppliers">Ir para Fornecedores</Link>
        </Button>
      </div>

      <Suspense>
        <InventoryToolbar
          accounts={accounts ?? []}
          selected={accountIds}
          from={period.date_from}
          to={period.date_to}
        />
      </Suspense>

      {accountIds.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>Escolha uma conta</EmptyTitle>
            <EmptyDescription>
              O estoque é por conta. Selecione uma conta com estoque próprio para ver os lotes e o
              fechamento do período.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="flex flex-col gap-6">
          <InventoryClosingPanel
            closing={closing}
            startsOn={period.date_from}
            endsOn={period.date_to}
          />
          <InventoryBatchesPanel
            accountId={accountIds[0]}
            batches={batches ?? []}
            origins={origins ?? []}
            agencies={agencies ?? []}
          />
          <StatementPanel
            accountId={accountIds[0]}
            origins={origins ?? []}
            agencies={agencies ?? []}
          />
        </div>
      )}
    </div>
  )
}
