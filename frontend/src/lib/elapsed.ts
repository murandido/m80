// The elapsed time since a connect login started, as a running "há 45s" / "há 1min 20s". The
// login legitimately takes a couple of minutes across edge retries, so a ticking counter reads
// as working where date-fns' coarse phrasing ("menos de um minuto") reads as stalled. Shared by
// the account card and the inventory statement panel, which both show the connecting state.
export function formatConnectingElapsed(sinceIso: string, nowMs: number): string {
  const since = new Date(sinceIso).getTime()
  if (Number.isNaN(since)) return ""
  const seconds = Math.max(0, Math.floor((nowMs - since) / 1000))
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return minutes > 0
    ? `há ${minutes}min ${String(remainder).padStart(2, "0")}s`
    : `há ${remainder}s`
}
