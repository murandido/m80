// Local-date helpers shared by every filter that talks date_from/date_to to the backend.
// They deliberately avoid `new Date("YYYY-MM-DD")`, which parses as UTC and can shift the
// day for anyone west of Greenwich - the operator's calendar day is what the backend
// compares against (it resolves the same strings in America/Sao_Paulo).

export function formatYmd(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

export function parseYmd(value: string): Date | undefined {
  if (!value) return undefined
  const [year, month, day] = value.split("-").map(Number)
  if (!year || !month || !day) return undefined
  return new Date(year, month - 1, day)
}

export function formatBr(value: string): string {
  const date = parseYmd(value)
  if (!date) return ""
  return date.toLocaleDateString("pt-BR")
}

export function startOfToday(): Date {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth(), now.getDate())
}

export function addDays(date: Date, days: number): Date {
  const copy = new Date(date)
  copy.setDate(copy.getDate() + days)
  return copy
}

// The period granularities the reports offer. Kept here rather than in a component so the
// Dashboard can reuse the same vocabulary when it lands. "custom" is the odd one out: it
// carries no shape of its own, the two ends of the range are what the user picked - the
// reconciliation of two or three days at a time is what it exists for.
export const PERIOD_PRESETS = ["day", "week", "month", "year", "custom"] as const

export type PeriodPreset = (typeof PERIOD_PRESETS)[number]

export const PERIOD_LABELS: Record<PeriodPreset, string> = {
  day: "Diário",
  week: "Semanal",
  month: "Mensal",
  year: "Anual",
  custom: "Personalizado",
}

export interface DateRangeParams {
  date_from: string
  date_to: string
}

// Expands a preset plus a reference day into the inclusive [date_from, date_to] the API
// expects. Weeks start on Monday, matching how the agency reports its numbers.
//
// "custom" has no expansion - its range lives in the URL's own date_from/date_to, which
// resolvePeriod prefers over whatever comes back from here. The reference day is returned
// so a custom period with nothing else in the URL still filters something narrow rather
// than falling through to every date.
export function resolvePeriodRange(
  preset: PeriodPreset,
  reference: Date,
): DateRangeParams {
  switch (preset) {
    // custom shares the day's expansion only as a fallback - see the note above.
    case "day":
    case "custom":
      return { date_from: formatYmd(reference), date_to: formatYmd(reference) }
    case "week": {
      // getDay() is 0 for Sunday, so Sunday has to walk back six days, not zero.
      const weekday = reference.getDay()
      const daysSinceMonday = weekday === 0 ? 6 : weekday - 1
      const monday = addDays(reference, -daysSinceMonday)
      return { date_from: formatYmd(monday), date_to: formatYmd(addDays(monday, 6)) }
    }
    case "month": {
      const first = new Date(reference.getFullYear(), reference.getMonth(), 1)
      const last = new Date(reference.getFullYear(), reference.getMonth() + 1, 0)
      return { date_from: formatYmd(first), date_to: formatYmd(last) }
    }
    case "year": {
      const first = new Date(reference.getFullYear(), 0, 1)
      const last = new Date(reference.getFullYear(), 11, 31)
      return { date_from: formatYmd(first), date_to: formatYmd(last) }
    }
  }
}

// Human label for a resolved range, e.g. "24/07/2026" or "01/07/2026 – 31/07/2026".
export function dateRangeLabel(from: string, to: string): string {
  if (!from && !to) return "Qualquer data"
  if (from && to && from === to) return formatBr(from)
  if (from && to) return `${formatBr(from)} – ${formatBr(to)}`
  if (from) return `A partir de ${formatBr(from)}`
  return `Até ${formatBr(to)}`
}
