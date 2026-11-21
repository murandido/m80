import { normalizeSearch } from "@/lib/search"
import { AccountOption } from "@/schemas/account"
import { PersonOption } from "@/schemas/person"

// An account has no name of its own. Its main name is its holder's, falling back to the
// login, then a registered alias, then a short id. This is the name every screen shows once
// an account is picked: the aliases are only ways of finding it, and showing whichever one
// matched made the same account read as several different ones.
export function accountMainName(
  account: AccountOption,
  people: PersonOption[] | Map<string, PersonOption>,
): string {
  const holder =
    account.holder_id === null
      ? undefined
      : people instanceof Map
        ? people.get(account.holder_id)
        : people.find((person) => person.id === account.holder_id)
  return (
    holder?.name ??
    account.login ??
    account.nicknames[0]?.nickname ??
    account.id.slice(0, 8)
  )
}

// How well one text answers what was typed, as a single sortable number. The tier comes
// first (equal, starts with, a word starts with, contains); within a tier, the text whose
// length is closest to the query wins - which is what puts "Ana - R" ahead of
// "Ana - Renata" for "Ana -", since the operator is visibly typing towards the shorter one.
const TIER_WEIGHT = 1000
const WORD_BOUNDARY = /[\s\-()/.,]+/

function textScore(text: string, needle: string): number {
  const haystack = normalizeSearch(text)
  let tier = 0
  if (haystack === needle) tier = 4
  else if (haystack.startsWith(needle)) tier = 3
  else if (haystack.split(WORD_BOUNDARY).some((word) => word.startsWith(needle)))
    tier = 2
  else if (haystack.includes(needle)) tier = 1
  if (tier === 0) return 0
  const lengthGap = Math.min(Math.abs(haystack.length - needle.length), TIER_WEIGHT - 1)
  return tier * TIER_WEIGHT - lengthGap
}

const EXACT_SCORE = 4 * TIER_WEIGHT
// Larger than any single text score, so an alias tier always outranks any fallback score.
const FALLBACK_SPAN = 5 * TIER_WEIGHT

export interface AliasMatch {
  id: string
  nickname: string
}

export interface AccountMatch {
  account: AccountOption
  mainName: string
  // The aliases worth showing under the main name: those that answer the query, closest
  // first, or every alias in registration order when nothing was typed. An alias spelled
  // like the main name is left out - it would only repeat the line above it.
  aliases: AliasMatch[]
  // The listed alias that answers the query best. Aliases are what operators type, so the
  // closest one is highlighted even when the main name matches too; null when no listed
  // alias answers.
  highlightedAliasId: string | null
  // The alias an issuance picked through this match should record. The best-answering one
  // when there is one, otherwise the alias spelled like the main name, otherwise the oldest: the
  // EMISSAO message names the account by alias, because that is what the bot resolves.
  nicknameId: string | null
  // True when the main name or an alias equals the query (ignoring case and accents).
  exact: boolean
  score: number
}

function buildMatch(
  account: AccountOption,
  mainName: string,
  needle: string,
  extraTexts: string[],
): AccountMatch | null {
  const normalizedMainName = normalizeSearch(mainName)
  const aliases = account.nicknames.filter(
    (entry) => normalizeSearch(entry.nickname) !== normalizedMainName,
  )
  const aliasSpelledLikeMainName = account.nicknames.find(
    (entry) => normalizeSearch(entry.nickname) === normalizedMainName,
  )
  const defaultNicknameId =
    aliasSpelledLikeMainName?.id ?? account.nicknames[0]?.id ?? null

  if (needle === "") {
    return {
      account,
      mainName,
      aliases,
      highlightedAliasId: null,
      nicknameId: defaultNicknameId,
      exact: false,
      score: 0,
    }
  }

  const mainScore = textScore(mainName, needle)
  const scoredAliases = account.nicknames
    .map((entry) => ({ entry, score: textScore(entry.nickname, needle) }))
    .filter((candidate) => candidate.score > 0)
    .sort((a, b) => b.score - a.score)
  const visibleAliases = scoredAliases.filter(
    (candidate) => normalizeSearch(candidate.entry.nickname) !== normalizedMainName,
  )
  const aliasScore = scoredAliases[0]?.score ?? 0
  const fallbackScore = Math.max(
    mainScore,
    ...extraTexts.map((text) => textScore(text, needle)),
  )
  if (aliasScore === 0 && fallbackScore === 0) return null

  // Aliases are what operators search by; the full name (and the program) only break ties
  // between accounts whose aliases answer equally, or find an account no alias answers.
  // Weighted into one number so the sort stays a single comparison.
  const score = aliasScore * FALLBACK_SPAN + fallbackScore
  const bestVisibleAlias = visibleAliases[0]?.entry ?? null
  return {
    account,
    mainName,
    aliases: visibleAliases.map((candidate) => candidate.entry),
    highlightedAliasId: bestVisibleAlias?.id ?? null,
    // The alias that answered best, even the one hidden for repeating the main name.
    nicknameId: scoredAliases[0]?.entry.id ?? defaultNicknameId,
    exact: mainScore >= EXACT_SCORE || aliasScore >= EXACT_SCORE,
    score,
  }
}

// The accounts that answer the query, best first, one entry per account. With nothing
// typed every account is returned in the order given. `extraTexts` adds searchable text
// that is not a name - the program's name and code in the filter bars.
export function rankAccountMatches(
  accounts: AccountOption[],
  people: PersonOption[],
  query: string,
  extraTexts?: (account: AccountOption) => string[],
): AccountMatch[] {
  const needle = normalizeSearch(query)
  // Built per call rather than scanned per account: this runs on every keystroke over the
  // whole, unpaginated account feed.
  const peopleById = new Map(people.map((person) => [person.id, person]))
  const matches: AccountMatch[] = []
  for (const account of accounts) {
    const match = buildMatch(
      account,
      accountMainName(account, peopleById),
      needle,
      extraTexts?.(account) ?? [],
    )
    if (match) matches.push(match)
  }
  if (needle === "") return matches
  return matches.sort(
    (a, b) => b.score - a.score || a.mainName.localeCompare(b.mainName, "pt-BR"),
  )
}

// The account whose main name or alias is exactly what was typed, if any. Used to reuse an
// account instead of registering a second one under a name it already answers to.
export function findExactAccountMatch(
  accounts: AccountOption[],
  people: PersonOption[],
  typed: string,
): AccountMatch | undefined {
  if (normalizeSearch(typed) === "") return undefined
  return rankAccountMatches(accounts, people, typed).find((match) => match.exact)
}
