import {
  PERIOD_PRESETS,
  type PeriodPreset,
  formatYmd,
  parseYmd,
  resolvePeriodRange,
  startOfToday,
} from "@/lib/date-range"

// The URL keys the period control owns, on every page that uses it. `period`/`reference`
// are what the user picked; date_from/date_to are the expansion the API actually filters
// on, kept in the URL so a shared link reproduces the exact range.
export const PERIOD_KEYS = ["period", "reference", "date_from", "date_to"] as const

export type PeriodSearchParams = Partial<
  Record<(typeof PERIOD_KEYS)[number], string>
>

export interface ResolvedPeriod {
  preset: PeriodPreset
  reference: string
  date_from: string
  date_to: string
}

export function isPeriodPreset(value: string | undefined): value is PeriodPreset {
  return PERIOD_PRESETS.includes(value as PeriodPreset)
}

// Expands the period params into the range the backend filters on. The default preset is
// the caller's to choose: "who still owes me money" wants a month, a live dashboard wants
// today.
export function resolvePeriod(
  params: PeriodSearchParams,
  defaultPreset: PeriodPreset,
): ResolvedPeriod {
  const preset = isPeriodPreset(params.period) ? params.period : defaultPreset
  const referenceDate = parseYmd(params.reference ?? "") ?? startOfToday()
  const range = resolvePeriodRange(preset, referenceDate)

  return {
    preset,
    reference: formatYmd(referenceDate),
    // An explicit date_from/date_to in the URL wins, so a hand-edited range survives a
    // reload instead of snapping back to the preset's expansion.
    date_from: params.date_from || range.date_from,
    date_to: params.date_to || range.date_to,
  }
}
