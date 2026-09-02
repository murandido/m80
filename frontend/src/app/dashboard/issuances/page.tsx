import type { Metadata } from "next"
import { Suspense } from "react"
import { cookies } from "next/headers"
import { AlertTriangle } from "lucide-react"
import { env } from "@/lib/env"
import { fetchList } from "@/lib/fetch-list"
import { readStickyFilters, stickySearchParams } from "@/lib/sticky-filters"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { getIssuancesSchema } from "@/schemas/issuance"
import { getLoyaltyProgramsSchema } from "@/schemas/loyalty-program"
import { getAirlinesSchema } from "@/schemas/airline"
import { getAccountsSchema } from "@/schemas/account"
import { getAgenciesSchema } from "@/schemas/agency"
import { getUsersSchema, getMeSchema } from "@/schemas/user"
import { getPeopleSchema } from "@/schemas/person"
import { getAirportsSchema } from "@/schemas/airport"
import { IssuancesGrid } from "@/components/issuances/issuances-grid"
import { IssuancesFilterBar } from "@/components/issuances/issuances-filter-bar"
import { ListSearchBar } from "@/components/shared/list-search-bar"

export const metadata: Metadata = {
  title: "Emissões",
  description:
    "Registre e acompanhe cada emissão, do localizador ao valor cobrado do cliente.",
}

// The filter dimensions the page understands, carried as URL query params.
const FILTER_KEYS = [
  "date_from",
  "date_to",
  "loyalty_program_id",
  "airline_id",
  "account_id",
  "agency_id",
  "issuer_id",
] as const

type FilterState = Record<(typeof FILTER_KEYS)[number], string>

// `q` is deliberately outside FILTER_KEYS: it must not count as one of the keys
// that switch the page off its "today + me" defaults, and it is not part of the
// sticky-restore set either. A pasted /dashboard/issuances?q=ABC therefore still
// gets the remembered dimension filters - and the backend ignores date and issuer
// while a search is active anyway.
type IssuancesSearchParams = Partial<Record<(typeof FILTER_KEYS)[number], string>> & {
  q?: string
}

async function fetchCurrentUserId(authHeader: Record<string, string>): Promise<string | null> {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/auth/me`, { headers: authHeader })
    const parsed = getMeSchema.response.safeParse(await res.json())
    if (!res.ok || !parsed.success || !parsed.data.data) return null
    return parsed.data.data.id
  } catch {
    return null
  }
}

// created_at is compared in America/Sao_Paulo on the backend, so "today" must be the
// operator's local calendar day, not the server's UTC day. en-CA renders as YYYY-MM-DD.
function todayInSaoPaulo(): string {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "America/Sao_Paulo",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date())
}

// A clean URL (no filter keys at all) gets the "today + current user" defaults.
// Once any key is present, the URL is authoritative and each absent key means that
// dimension is unfiltered - so clearing one filter never snaps back to the defaults.
function resolveFilters(
  params: IssuancesSearchParams,
  currentUserId: string | null,
): FilterState {
  const hasAnyFilterParam = FILTER_KEYS.some((key) => params[key] !== undefined)

  if (!hasAnyFilterParam) {
    const today = todayInSaoPaulo()
    return {
      date_from: today,
      date_to: today,
      loyalty_program_id: "",
      airline_id: "",
      account_id: "",
      agency_id: "",
      issuer_id: currentUserId ?? "",
    }
  }

  return {
    date_from: params.date_from ?? "",
    date_to: params.date_to ?? "",
    loyalty_program_id: params.loyalty_program_id ?? "",
    airline_id: params.airline_id ?? "",
    account_id: params.account_id ?? "",
    agency_id: params.agency_id ?? "",
    issuer_id: params.issuer_id ?? "",
  }
}

export default async function IssuancesPage({
  searchParams,
}: {
  searchParams: Promise<IssuancesSearchParams>
}) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const authHeader = { Authorization: `Bearer ${token}` }
  const api = `${env.BACKEND_URL}/api/v1`

  // First wave: everything that does not depend on the resolved filters. Each
  // promise resolves (never rejects), so a single failure degrades only its own list.
  const [currentUserId, loyaltyPrograms, airlines, accounts, agencies, users, people, airports] =
    await Promise.all([
      fetchCurrentUserId(authHeader),
      fetchList(`${api}/loyalty-programs`, getLoyaltyProgramsSchema.response, authHeader),
      fetchList(`${api}/airlines`, getAirlinesSchema.response, authHeader),
      fetchList(`${api}/accounts`, getAccountsSchema.response, authHeader),
      fetchList(`${api}/agencies`, getAgenciesSchema.response, authHeader),
      fetchList(`${api}/users`, getUsersSchema.response, authHeader),
      fetchList(`${api}/people`, getPeopleSchema.response, authHeader),
      fetchList(`${api}/airports/all`, getAirportsSchema.response, authHeader),
    ])

  // Second wave: the filtered issuance list, which needs currentUserId to build the defaults.
  // A bare URL (every sidebar link is one) falls back to the filters this browser session
  // last used here, so the screen is already filtered on the first paint.
  const requested = await searchParams
  const filters = resolveFilters(
    stickySearchParams(
      requested,
      readStickyFilters(cookieStore, "/dashboard/issuances"),
      FILTER_KEYS,
    ),
    currentUserId,
  )
  // Read straight off the request: stickySearchParams only ever returns the keys it
  // was asked about, so routing `q` through it would drop the search on exactly the
  // URL that carries nothing else.
  const search = (requested.q ?? "").trim()
  const issuanceQuery = new URLSearchParams()
  for (const key of FILTER_KEYS) {
    if (filters[key]) issuanceQuery.set(key, filters[key])
  }
  // Sent alongside the dimension filters even though the backend drops date and
  // issuer while it is present: clearing the search has to restore them, so their
  // values stay in the URL rather than being erased here.
  if (search) issuanceQuery.set("q", search)
  const issuancesUrl = issuanceQuery.toString()
    ? `${api}/issuances?${issuanceQuery.toString()}`
    : `${api}/issuances`

  const issuances = await fetchList(
    issuancesUrl,
    getIssuancesSchema.response,
    authHeader,
    "no-store",
  )

  const referenceDataFailed = [
    loyaltyPrograms,
    airlines,
    accounts,
    agencies,
    users,
    people,
    airports,
  ].some((list) => list === null)

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-2 sm:gap-6">
        {/* The search box sits here, outside <IssuancesGrid>, on purpose: the grid is
            keyed on the resolved filters and remounts on every filter change, taking
            the filter bar with it. A text input in that subtree would lose focus each
            time the debounce fired. As a sibling it survives the remount. */}
        <div className="flex flex-wrap items-end justify-between gap-3">
          <h1 className="text-lg font-semibold text-slate-800">Emissões</h1>
          <div className="w-full sm:w-96">
            <Suspense fallback={<div className="h-14" />}>
              <ListSearchBar placeholder="PNR, passageiro, origem ou destino" />
            </Suspense>
          </div>
        </div>

        {issuances === null ? (
          // The grid is deliberately not rendered: showing an empty spreadsheet after a
          // failed load invites the user to retype issuances that already exist.
          <Alert variant="destructive">
            <AlertTriangle />
            <AlertTitle>Não foi possível carregar as emissões</AlertTitle>
            <AlertDescription>
              As emissões já cadastradas não puderam ser lidas do servidor. Recarregue a página —
              cadastrar agora poderia duplicar emissões existentes.
            </AlertDescription>
          </Alert>
        ) : (
          <>
            {referenceDataFailed && (
              <Alert>
                <AlertTriangle />
                <AlertTitle>Algumas listas não carregaram</AlertTitle>
                <AlertDescription>
                  Companhias, contas, agências ou emissores podem aparecer incompletos nos campos de
                  seleção. Recarregue a página para tentar de novo.
                </AlertDescription>
              </Alert>
            )}

            <IssuancesGrid
              // Keyed on the resolved filters so a client-side filter change (router.replace,
              // which re-renders this Server Component but does not remount the grid) forces a
              // remount. Without it the grid seeds `rows` from initialIssuances only on first
              // mount and keeps showing the previous filter's issuances until a full reload.
              key={[...FILTER_KEYS.map((key) => filters[key]), search].join("|")}
              filters={
                <IssuancesFilterBar
                  loyaltyPrograms={loyaltyPrograms ?? []}
                  airlines={airlines ?? []}
                  accounts={accounts ?? []}
                  people={people ?? []}
                  agencies={agencies ?? []}
                  users={users ?? []}
                  initialFilters={filters}
                  hasSearch={search !== ""}
                />
              }
              initialIssuances={issuances}
              filterQuery={issuanceQuery.toString()}
              loyaltyPrograms={loyaltyPrograms ?? []}
              airlines={airlines ?? []}
              accounts={accounts ?? []}
              agencies={agencies ?? []}
              users={users ?? []}
              people={people ?? []}
              airports={airports ?? []}
              currentUserId={currentUserId}
            />
          </>
        )}
      </div>
    </div>
  )
}
