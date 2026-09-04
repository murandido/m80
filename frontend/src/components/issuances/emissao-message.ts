import { IssuanceRow } from "@/components/issuances/types"
import { AccountOption } from "@/schemas/account"
import { Agency } from "@/schemas/agency"
import { Airline } from "@/schemas/airline"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { PersonOption } from "@/schemas/person"

// Rebuilds the WhatsApp EMISSAO message from a row of the grid - the inverse of the bot's
// parser (backend/internal/bot/emissao_parse.go) and of resolveEmissao, which is what turns
// a parsed message into an issuance. Everything about the format lives in this one file so
// it can be read side by side with the Go, which is the only way the two stay in step.
//
//   EMISSAO - SMILES - IWBEQP
//   CNF - MACHADO
//   31k * 16,5 + 189,22 tx = 700,72
//   Heraldo
//
// The fifth line (the observation preset code) is deliberately not rebuilt: the issuance
// stores the preset's resolved *text*, not its code, so there is nothing here to invert.

/** Reference lists needed to turn the row's ids back into the names an operator types. */
export interface EmissaoReferenceData {
  loyaltyPrograms: LoyaltyProgram[]
  airlines: Airline[]
  accounts: AccountOption[]
  agencies: Agency[]
  people: PersonOption[]
}

// `missing` carries column labels straight from the grid header, so the caller can say
// which cells to fill instead of a bare "could not copy".
export type EmissaoText = { text: string } | { missing: string[] }

/**
 * Points as the bot writes them: 31000 -> "31k", 89912 -> "89,912k".
 * Same transform the Pontos cell uses while editing (cells/number-cell.tsx).
 */
export function formatPointsK(points: number): string {
  return `${String(points / 1000).replace(".", ",")}k`
}

/**
 * Centavos as the bot writes them: 70072 -> "700,72", 4 -> "0,04".
 *
 * Mirrors formatCents in emissao_parse.go rather than reusing formatCurrencyBRL, and the
 * difference is not cosmetic: formatCurrencyBRL renders 150000 as "R$ 1.500,00", and the
 * parser rejects a thousands-dotted amount (ErrTooManyDecimals). A message formatted that
 * way would not survive the round trip.
 */
export function formatPlainCents(cents: number): string {
  const reais = Math.trunc(cents / 100)
  const centavos = Math.abs(cents % 100)
  return `${reais},${String(centavos).padStart(2, "0")}`
}

/**
 * The "Valor total" of a row, in centavos. Null when the two required numbers are missing.
 *
 * Shared with the calculated column on purpose. The bot re-derives this number and refuses
 * the message when it disagrees with the one written on line 3 (validateTotal), so the
 * total on screen and the total in the copied text have to come from the same expression.
 */
export function issuanceTotalCents(
  row: Pick<IssuanceRow, "points" | "value_of_the_thousand" | "taxes">,
): number | null {
  if (row.points === null || row.value_of_the_thousand === null) return null
  return Math.round((row.points / 1000) * row.value_of_the_thousand) + (row.taxes ?? 0)
}

/**
 * How to name an account that has no registered alias - used both by the account combobox
 * and by line 4 of the message, which is why it lives here rather than in either of them.
 */
export function accountFallbackLabel(account: AccountOption, people: PersonOption[]): string {
  const holder = people.find((p) => p.id === account.holder_id)
  return holder?.name ?? account.login ?? account.id
}

/**
 * The price per thousand, with trailing zeros trimmed: 1650 -> "16,5", 3000 -> "30".
 *
 * Every real message in the bot's fixtures writes it this way, while taxes and the total
 * always keep both digits - a milheiro is quoted the way it is spoken ("trinta"), the other
 * two are exact amounts of money. The parser accepts either, so this is about the message
 * reading like one a person typed.
 */
export function formatMilheiro(cents: number): string {
  return formatPlainCents(cents)
    .replace(/,00$/, "")
    .replace(/(,\d)0$/, "$1")
}

/**
 * The surname alone, which is all line 2 carries: "Joao Machado" -> "Machado".
 *
 * The two ends of this field disagree, and the message has to follow the bot's end. What
 * the bot parses off line 2 is a surname (assignLine2Extras stores exactly that), but the
 * "Passageiro principal" column is free text, so an operator filling the grid by hand types
 * the whole name. Sending that back would put a first name where the airline expects a
 * surname. A single-word value is already a surname and passes through untouched.
 */
export function lastNameOf(name: string | null): string | null {
  if (!name) return null
  const parts = name.trim().split(/\s+/)
  return parts[parts.length - 1] || null
}

/** Line 3 on its own: "31k * 16,5 + 189,22 tx = 700,72". */
export function buildCalculo(row: IssuanceRow): EmissaoText {
  const { points, value_of_the_thousand: valueOfTheThousand, taxes } = row

  const missing: string[] = []
  if (points === null) missing.push("Pontos")
  if (valueOfTheThousand === null) missing.push("Valor milheiro")

  const total = issuanceTotalCents(row)
  if (points === null || valueOfTheThousand === null || total === null) return { missing }

  // No "+ N tx" when there is no tax at all: the parser reads a missing "+" term as zero,
  // and "+ 0,00 tx" is not something an operator would type.
  const taxTerm = taxes === null ? "" : ` + ${formatPlainCents(taxes)} tx`

  return {
    text: `${formatPointsK(points)} * ${formatMilheiro(valueOfTheThousand)}${taxTerm} = ${formatPlainCents(total)}`,
  }
}

/** The whole four-line message. */
export function buildEmissaoResumo(row: IssuanceRow, ref: EmissaoReferenceData): EmissaoText {
  const program = ref.loyaltyPrograms.find((p) => p.id === row.loyalty_program_id)
  const account = ref.accounts.find((a) => a.id === row.account_id)

  // The airline whose rules apply, which is not always the one written on line 1: a row
  // that never picked one flies on the program's default, and that default's flags are
  // what resolveEmissao reads.
  const airline = ref.airlines.find(
    (a) => a.id === (row.airline_id ?? program?.default_airline_id),
  )

  const missing: string[] = []
  if (!program) missing.push("Cia aérea")
  if (row.program_locator.trim() === "") missing.push("Localizador")
  if (!row.origin_iata_code) missing.push("Origem")
  // Only when the airline demands it. Leaving it out would produce a message the bot
  // refuses outright ("A LATAM exige o sobrenome do passageiro na linha 2"), so this is
  // worth blocking on rather than copying and finding out at the other end.
  if (airline?.requires_last_name && !row.main_passenger_name) {
    missing.push("Passageiro principal")
  }
  if (airline?.requires_purchase_number && !row.latam_purchase_number) {
    missing.push("Número da compra")
  }
  if (row.points === null) missing.push("Pontos")
  if (row.value_of_the_thousand === null) missing.push("Valor milheiro")
  if (!account) missing.push("Conta")
  if (missing.length > 0 || !program || !account) return { missing }

  const calculo = buildCalculo(row)
  if ("missing" in calculo) return calculo

  // The airline is only written out when it differs from the program's default, because
  // that is exactly when resolveEmissao stores one - and it is what an operator types.
  const explicitAirline =
    row.airline_id && row.airline_id !== program.default_airline_id ? airline : undefined

  // Locators are upper-cased for the same reason line 2 is: the parser does it, and unlike
  // the airport cell the locator cell does not normalise what is typed into it.
  const locator = row.program_locator.trim().toUpperCase()
  const header = [
    "EMISSAO",
    explicitAirline ? `${program.code}/${explicitAirline.code}` : program.code,
    row.airline_locator ? `${locator}/${row.airline_locator.trim().toUpperCase()}` : locator,
  ].join(" - ")

  // What follows the origin is decided by the airline's flags, not by what happens to be
  // stored. assignLine2Extras reads these two positionally and only when the airline asks
  // for them, so writing a surname AZUL never wanted would be one field too many - and the
  // parser, seeing a volunteered extra, would take it as a surname anyway.
  //
  // Upper-cased because the parser upper-cases all of them, so this is the exact inverse.
  const route = [
    row.origin_iata_code,
    airline?.requires_last_name ? lastNameOf(row.main_passenger_name) : null,
    airline?.requires_purchase_number ? row.latam_purchase_number : null,
  ]
    .filter((part): part is string => Boolean(part))
    .map((part) => part.toUpperCase())
    .join(" - ")

  const nickname = account.nicknames.find((n) => n.id === row.account_nickname_id)
  const agency = ref.agencies.find((a) => a.id === row.agency_id)
  // The default agency is what the bot falls back to when line 4 has no parentheses, so
  // naming it would only add noise to a message that already means the same thing.
  const agencySuffix =
    agency && !agency.is_default
      ? ` (${agency.nicknames[0]?.nickname ?? agency.name})`
      : ""
  // The administrator (line 4's optional middle field) is left out: in the bot it is an
  // instruction to record who administers the account, not a description of the issuance.
  const owners = `${nickname?.nickname ?? accountFallbackLabel(account, ref.people)}${agencySuffix}`

  return { text: [header, route, calculo.text, owners].join("\n") }
}
