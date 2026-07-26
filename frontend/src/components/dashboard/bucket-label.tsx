const MONTH_NAMES = [
  "jan",
  "fev",
  "mar",
  "abr",
  "mai",
  "jun",
  "jul",
  "ago",
  "set",
  "out",
  "nov",
  "dez",
]

// Turns a server bucket key into an axis label. The key is parsed by hand rather than with
// `new Date(...)`, which reads a bare "YYYY-MM-DD" as UTC and would shift the label a day
// back for anyone west of Greenwich - the same trap src/lib/date-range.ts documents.
export function formatBucketLabel(bucket: string, granularity: string): string {
  if (granularity === "hour") {
    const [, time] = bucket.split("T")
    return time ?? bucket
  }

  if (granularity === "month") {
    const [, month] = bucket.split("-")
    const index = Number(month) - 1
    return MONTH_NAMES[index] ?? bucket
  }

  const [, month, day] = bucket.split("-")
  if (!month || !day) return bucket
  return `${day}/${month}`
}
