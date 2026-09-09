import type { Metadata } from "next"
import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { requireAdmin } from "@/lib/session"
import { fetchList } from "@/lib/fetch-list"
import { getPointBatchOriginsSchema, type PointBatchOrigin } from "@/schemas/inventory"
import { PointBatchOriginsScreen } from "@/components/inventory/point-batch-origins-screen"

export const metadata: Metadata = {
  title: "Origens de lote",
}

export default async function PointBatchOriginsPage() {
  await requireAdmin()

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  // Every origin, active and retired: this is the screen where a retired one is brought
  // back, so hiding them here would make that impossible.
  const origins = await fetchList<PointBatchOrigin>(
    `${env.BACKEND_URL}/api/v1/point-batch-origins`,
    getPointBatchOriginsSchema.response,
    { Authorization: `Bearer ${token}` },
  )

  return (
    <div className="flex flex-col gap-6 p-4 md:p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Origens de lote</h1>
        <p className="text-sm text-slate-500">
          De onde vêm os pontos: parceiro de transferência, banco, compra direta na companhia,
          clube ou acúmulo de voo.
        </p>
      </div>

      <PointBatchOriginsScreen origins={origins ?? []} />
    </div>
  )
}
