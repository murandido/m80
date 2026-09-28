"use client"

import { useIsQueryNavigationPending } from "@/hooks/use-url-query"

// A thin bar across the top while a filter change is on its way to the server. The
// controls already show the new pick, but the list under them stays the old one until the
// answer lands - a second or two in production - and without a signal that reads as a
// filter that did nothing.
export function QueryNavigationProgress() {
  const isPending = useIsQueryNavigationPending()
  if (!isPending) return null

  return (
    <div
      role="progressbar"
      aria-label="Atualizando a lista"
      className="pointer-events-none fixed inset-x-0 top-0 z-50 h-0.5 overflow-hidden opacity-0"
      style={{ animation: "query-progress-appear 150ms ease-out 200ms forwards" }}
    >
      <div
        className="h-full w-2/5 bg-slate-800"
        style={{ animation: "query-progress-slide 1s ease-in-out infinite" }}
      />
    </div>
  )
}
