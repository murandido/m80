import type { Metadata } from "next"
import { cookies } from "next/headers"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { DirectLookupsTable } from "@/components/airlines/direct-lookups-table"
import { env } from "@/lib/env"
import { requireDev } from "@/lib/session"
import { cn } from "@/lib/utils"
import {
  directLookupReportResponseSchema,
  type AirlineLookupReport,
} from "@/schemas/booking-lookup"

// Temporary: this screen exists to decide when each airline can drop Traveos, and goes
// away with the vendor, together with GET /booking-lookups/report and migration 66.

export const metadata: Metadata = {
  title: "Conexões diretas",
  description:
    "Quantas buscas de itinerário cada companhia respondeu sozinha e quantas caíram para a Traveos.",
}

const WINDOWS = [7, 30, 90] as const

export default async function DirectLookupsPage({
  searchParams,
}: {
  searchParams: Promise<{ days?: string }>
}) {
  await requireDev()

  const requested = Number((await searchParams).days)
  const days = WINDOWS.find((window) => window === requested) ?? WINDOWS[0]

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  let airlines: AirlineLookupReport[] | null = null
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/booking-lookups/report?days=${days}`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const parsed = directLookupReportResponseSchema.safeParse(await res.json())
    if (res.ok && parsed.success) {
      airlines = parsed.data.data?.airlines ?? []
    }
  } catch {
    // airlines stays null and the page says the report could not be read
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-lg font-semibold text-slate-800">Conexões diretas</h1>
          <p className="text-sm text-slate-500">
            Cada busca de itinerário, de emissão pelo painel ou pelo bot, pergunta primeiro à
            companhia quando a conexão direta está ligada, e cai para a Traveos quando ela não
            responde. Com a conexão desligada, só a Traveos é consultada. Uma companhia está
            pronta para deixar a Traveos quando nada mais cai para ela.
          </p>
        </div>

        <div className="flex gap-2">
          {WINDOWS.map((window) => (
            <Button
              key={window}
              asChild
              size="sm"
              variant={window === days ? "default" : "outline"}
              className={cn(window === days && "bg-slate-800 hover:bg-slate-900")}
            >
              <Link href={`/dashboard/airlines/direct-lookups?days=${window}`}>
                Últimos {window} dias
              </Link>
            </Button>
          ))}
        </div>

        {airlines === null ? (
          <p className="text-sm text-red-600">Não foi possível carregar o relatório.</p>
        ) : airlines.length === 0 ? (
          <p className="text-sm text-slate-500">
            Nenhuma busca de itinerário registrada nesse período.
          </p>
        ) : (
          <DirectLookupsTable airlines={airlines} />
        )}
      </div>
    </div>
  )
}
