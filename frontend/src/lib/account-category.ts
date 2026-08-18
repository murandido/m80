import type { AccountCategory, CPFUsage } from "@/schemas/account"

// The API stores the categories in English, like every other enum in the schema. What the
// operator reads is here, spelled out in Portuguese - "Liminar" is the word they use for an
// account under a court order suspending the program's beneficiary cap.
export const ACCOUNT_CATEGORY_LABELS: Record<AccountCategory, string> = {
  unidentified: "Não identificado",
  standard: "Normal",
  injunction: "Liminar",
}

export const ACCOUNT_CATEGORY_OPTIONS = (
  Object.keys(ACCOUNT_CATEGORY_LABELS) as AccountCategory[]
).map((value) => ({ value, label: ACCOUNT_CATEGORY_LABELS[value] }))

export function accountCategoryLabel(category: string): string {
  return ACCOUNT_CATEGORY_LABELS[category as AccountCategory] ?? category
}

// The cap is counted in CPFs on every program, whatever the mode. What changes between them
// is whether a CPF already used can be reused - LATAM charges a fresh slot for a passenger it
// already flew, Smiles does not - and that is a rule about reuse, not a different unit. The
// hover below is where the difference is spelled out.
export function cpfUsageNoun(count: number): string {
  return count === 1 ? "CPF" : "CPFs"
}

// Amber from four fifths of the cap, red once it is reached. The thresholds are proportional
// rather than absolute because the caps differ by an order of magnitude - Azul allows five,
// Smiles twenty-five, and "two left" means something very different in each.
export function cpfUsageSeverity(usage: CPFUsage): "ok" | "close" | "over" {
  if (usage.remaining <= 0) return "over"
  if (usage.limit > 0 && usage.used / usage.limit >= 0.8) return "close"
  return "ok"
}

// Everything the hover has to say, because the number alone promises more than it can
// deliver: it says which window it counts, when it resets, and - only where the count is by
// distinct person - that a passenger with no CPF on file is matched by name and may be
// undercounted.
export function cpfUsageExplanation(usage: CPFUsage): string {
  const window =
    usage.window === "calendar_year"
      ? "Conta o ano civil, e zera em 1º de janeiro."
      : "Conta os últimos 12 meses, deslizando dia a dia."

  const counting =
    usage.mode === "per_issuance"
      ? "Um CPF não pode ser reaproveitado: emitir de novo para a mesma pessoa consome outra vaga."
      : "Um CPF já usado pode ser reaproveitado: emitir de novo para a mesma pessoa não consome vaga nova."

  const caveat =
    usage.mode === "per_issuance"
      ? ""
      : " Sem CPF, o passageiro é identificado pelo nome — o número pode estar subestimado."

  return `${window} ${counting}${caveat}`
}
