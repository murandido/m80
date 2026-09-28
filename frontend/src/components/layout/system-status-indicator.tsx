"use client"

import { useCallback, useState, useTransition } from "react"
import Link from "next/link"
import { Bot as BotIcon, Plane, PlaneTakeoff } from "lucide-react"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import { useSSE } from "@/hooks/use-sse"
import { getSystemStatusAction } from "@/actions/status"
import type { DependencyStatus, SystemStatus } from "@/schemas/status"

// One light per dependency, always visible in the header. The point is that
// the person working the issuances grid all day finds out the vendor is down
// from the panel, not from a ⚠️ on a WhatsApp message they can't explain.
//
// The airline integrations share a single aggregated light, because that set
// grows with every carrier moved off Traveos and this header has to keep fitting
// on a phone. Which one is down is in the popover.

type Tone = "ok" | "bad" | "warn" | "idle"

// Darkened one step from the -400 these used while the header was slate-700:
// on the light header the lighter shades failed contrast against white.
const DOT_CLASS: Record<Tone, string> = {
  ok: "bg-emerald-600",
  bad: "bg-red-600",
  warn: "bg-amber-600",
  // Grey covers both "never configured" and "not probed yet": neither is a
  // problem to act on, and a second alarming colour would dilute the red.
  idle: "bg-slate-400",
}

interface Light {
  tone: Tone
  label: string
}

// The bot has two failure modes that are independent: the process can be gone,
// or it can be running and logged out of WhatsApp. `state` answers the first,
// `session` the second, and only reading both tells the truth.
function botLight(status: DependencyStatus | undefined): Light {
  if (!status || status.state === "unknown") return { tone: "idle", label: "Verificando..." }
  if (status.state === "not_configured") return { tone: "idle", label: "Não configurado" }
  if (status.state === "down") return { tone: "bad", label: "Sem resposta" }

  switch (status.session) {
    case "connected":
      return { tone: "ok", label: "Conectado" }
    case "pairing":
      return { tone: "warn", label: "Pareando" }
    case "disconnected":
      return { tone: "bad", label: "Desconectado" }
    default:
      // The process answered but did not say which session state it is in.
      return { tone: "warn", label: "Estado desconhecido" }
  }
}

function traveosLight(status: DependencyStatus | undefined): Light {
  if (!status || status.state === "unknown") return { tone: "idle", label: "Verificando..." }
  if (status.state === "not_configured") return { tone: "idle", label: "Não configurada" }
  if (status.state === "down") return { tone: "bad", label: "Fora do ar" }
  return { tone: "ok", label: "Operando" }
}

// The airline integrations are named by the backend after the provider, not the
// carrier, because one of them can serve more than one airline - the GOL endpoint
// answers for bookings sold as Smiles too. Anything not listed here is shown as
// the backend spelled it, in caps: a new carrier must not wait on this file.
const PROVIDER_LABELS: Record<string, string> = {
  azul: "Azul",
  gol: "GOL",
  latam: "LATAM",
  american: "American",
  copa: "Copa",
  avianca: "Avianca",
  airfrance: "Air France",
  iberia: "Iberia",
  tap: "TAP",
  aerolineas: "Aerolíneas Argentinas",
}

function providerLabel(name: string): string {
  return PROVIDER_LABELS[name] ?? name.toUpperCase()
}

// One light for every airline integration at once, showing the worst of them.
//
// Aggregated rather than one light per carrier because this sits in a sticky
// header that has to survive a phone: the count grows with every carrier migrated
// off the vendor, and the detail is one click away in the popover. Null means
// there is nothing to draw - no integration is wired on this server yet, and an
// extra grey dot would only be asking a question nobody has an answer for.
function airlinesLight(providers: Record<string, DependencyStatus>): Light | null {
  const states = Object.values(providers)
  if (states.length === 0) return null

  const down = states.filter((status) => status.state === "down").length
  if (down > 0) {
    return {
      tone: "bad",
      label: down === states.length ? "Fora do ar" : `${down} de ${states.length} fora do ar`,
    }
  }
  if (states.some((status) => status.state === "unknown")) {
    return { tone: "idle", label: "Verificando..." }
  }
  if (states.every((status) => status.state === "not_configured")) {
    return { tone: "idle", label: "Não configuradas" }
  }
  return { tone: "ok", label: "Operando" }
}

// The panel says how old the answer is rather than implying it is live - the
// probes run every 60s (bot) and 120s (Traveos).
function checkedAgo(isoTimestamp: string | undefined): string {
  if (!isoTimestamp) return "nunca checado"

  const elapsedSeconds = Math.round((Date.now() - new Date(isoTimestamp).getTime()) / 1000)
  if (!Number.isFinite(elapsedSeconds) || elapsedSeconds < 0) return "agora"
  if (elapsedSeconds < 60) return `há ${elapsedSeconds}s`

  const minutes = Math.floor(elapsedSeconds / 60)
  if (minutes < 60) return `há ${minutes} min`
  return `há ${Math.floor(minutes / 60)} h`
}

function StatusDot({ tone }: { tone: Tone }) {
  return (
    <span
      className={cn(
        "h-2 w-2 shrink-0 rounded-full",
        DOT_CLASS[tone],
        // Only the "still checking" state pulses. A steady red is a fact; a
        // blinking one reads as something in progress.
        tone === "idle" && "animate-pulse",
      )}
    />
  )
}

export function SystemStatusIndicator({
  initialStatus,
  canSeeBot = false,
}: {
  initialStatus: SystemStatus | null
  canSeeBot?: boolean
}) {
  const [status, setStatus] = useState(initialStatus)
  const [, startTransition] = useTransition()

  // The SSE events carry no payload by convention - refetch the authoritative
  // state instead of trusting anything on the wire.
  const refresh = useCallback(() => {
    startTransition(async () => {
      const result = await getSystemStatusAction()
      if ("status" in result) {
        setStatus(result.status)
      }
    })
  }, [])

  // Two events, on purpose. "system.status" is the background probe noticing a
  // change, at most once per tick. "bot.status" is the bot itself announcing a
  // session change the instant it happens - without it, an admin logging the
  // bot out would leave a green light on everyone's screen for up to a minute.
  useSSE(
    "/api/events",
    { "system.status": refresh, "bot.status": refresh },
    { onReconnect: refresh },
  )

  const bot = botLight(status?.bot)
  const traveos = traveosLight(status?.traveos)
  const providers = status?.providers ?? {}
  const airlines = airlinesLight(providers)

  return (
    <Popover>
      <PopoverTrigger
        className="flex items-center gap-3 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
        aria-label="Status do sistema"
      >
        <span className="flex items-center gap-1.5">
          <StatusDot tone={bot.tone} />
          {/* The header is tight on phones, so the labels drop and the dots stay. */}
          <span className="hidden sm:inline">Bot</span>
        </span>
        <span className="flex items-center gap-1.5">
          <StatusDot tone={traveos.tone} />
          <span className="hidden sm:inline">Traveos</span>
        </span>
        {airlines && (
          <span className="flex items-center gap-1.5">
            <StatusDot tone={airlines.tone} />
            <span className="hidden sm:inline">Companhias</span>
          </span>
        )}
      </PopoverTrigger>

      <PopoverContent align="end" className="w-72">
        <div className="flex flex-col gap-3 text-sm">
          <div className="flex items-start gap-2">
            <BotIcon className="mt-0.5 h-4 w-4 shrink-0 text-slate-500" />
            <div className="flex flex-1 flex-col">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium text-slate-800">Bot do WhatsApp</span>
                <span className="flex items-center gap-1.5 text-slate-600">
                  <StatusDot tone={bot.tone} />
                  {bot.label}
                </span>
              </div>
              <span className="text-xs text-slate-400">
                checado {checkedAgo(status?.bot.checked_at)}
              </span>
            </div>
          </div>

          <div className="flex items-start gap-2">
            <Plane className="mt-0.5 h-4 w-4 shrink-0 text-slate-500" />
            <div className="flex flex-1 flex-col">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium text-slate-800">Traveos</span>
                <span className="flex items-center gap-1.5 text-slate-600">
                  <StatusDot tone={traveos.tone} />
                  {traveos.label}
                </span>
              </div>
              <span className="text-xs text-slate-400">
                checado {checkedAgo(status?.traveos.checked_at)}
              </span>
            </div>
          </div>

          {airlines && (
            <div className="flex items-start gap-2">
              <PlaneTakeoff className="mt-0.5 h-4 w-4 shrink-0 text-slate-500" />
              <div className="flex flex-1 flex-col gap-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-medium text-slate-800">Companhias</span>
                  <span className="flex items-center gap-1.5 text-slate-600">
                    <StatusDot tone={airlines.tone} />
                    {airlines.label}
                  </span>
                </div>
                {/* Sorted by name so the list does not reshuffle between renders -
                    the object arrives in whatever order the backend's map iterated. */}
                {Object.entries(providers)
                  .sort(([a], [b]) => a.localeCompare(b))
                  .map(([name, dependency]) => {
                    const light = traveosLight(dependency)
                    return (
                      <div key={name} className="flex items-center justify-between gap-2 text-xs">
                        <span className="text-slate-500">{providerLabel(name)}</span>
                        <span className="flex items-center gap-1.5 text-slate-400">
                          <StatusDot tone={light.tone} />
                          {light.label} · {checkedAgo(dependency.checked_at)}
                        </span>
                      </div>
                    )
                  })}
              </div>
            </div>
          )}

          {/* Two sentences, because on a server with no direct integration there
              is nothing to fall back *from*: talking about "as duas" there would
              describe an arrangement that does not exist on this deployment. */}
          {(traveos.tone === "bad" || airlines?.tone === "bad") && (
            <p className="text-xs text-slate-500">
              {airlines
                ? "Emissões continuam sendo registradas. Quando a companhia não responde, o itinerário é puxado pela Traveos; com as duas fora do ar, ele só entra quando uma delas voltar."
                : "Emissões continuam sendo registradas, mas o itinerário não é puxado até a Traveos voltar."}
            </p>
          )}

          {/* Only a dev has anywhere to go from here - pairing and logout live
              behind the dev-only /dashboard/bot. Everyone else still sees the light,
              which is the point: knowing the bot is down matters to the operator even
              when fixing it is not their job. */}
          {canSeeBot && (
            <Link
              href="/dashboard/bot"
              className="text-xs font-medium text-slate-600 underline underline-offset-2 hover:text-slate-800"
            >
              Configuração do bot
            </Link>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
