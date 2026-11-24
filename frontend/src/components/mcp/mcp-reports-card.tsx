import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { formatDateTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { MCPReport } from "@/schemas/mcp"

const SEVERITY_LABELS: Record<MCPReport["findings"][number]["severity"], string> = {
  critical: "Crítico",
  warning: "Atenção",
  info: "Info",
}

const SEVERITY_ORDER = ["critical", "warning", "info"] as const

function formatDay(day: string): string {
  const [year, month, date] = day.split("-")
  return `${date}/${month}/${year}`
}

// What the agents' routines found, one report per token per day. Server-rendered and plain
// <details>: there is nothing to edit here, only to read.
export function MCPReportsCard({ reports }: { reports: MCPReport[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Relatórios</CardTitle>
        <CardDescription>
          O que as rotinas do agente conferiram, encontraram e fizeram, um relatório por dia.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {reports.length === 0 && (
          <p className="py-6 text-center text-sm text-muted-foreground">Nenhum relatório ainda.</p>
        )}
        {reports.map((report, index) => {
          const counts = SEVERITY_ORDER.map((severity) => ({
            severity,
            count: report.findings.filter((finding) => finding.severity === severity).length,
          })).filter(({ count }) => count > 0)

          return (
            <details key={report.id} open={index === 0} className="rounded-md border">
              <summary className="flex cursor-pointer flex-wrap items-center gap-2 px-4 py-3 text-sm">
                <span className="font-medium">{formatDay(report.report_date)}</span>
                <span className="text-slate-800">{report.title}</span>
                {counts.map(({ severity, count }) => (
                  <Badge
                    key={severity}
                    variant={severity === "critical" ? "destructive" : "secondary"}
                    className={cn(severity === "warning" && "bg-amber-100 text-amber-900")}
                  >
                    {count} {SEVERITY_LABELS[severity]}
                  </Badge>
                ))}
                {counts.length === 0 && <Badge variant="secondary">Sem achados</Badge>}
                <span className="ml-auto text-xs text-muted-foreground">
                  {report.token_name ?? "—"} · {formatDateTime(report.updated_at)}
                </span>
              </summary>

              <div className="flex flex-col gap-4 border-t px-4 py-3 text-sm">
                <p className="whitespace-pre-wrap text-slate-700">{report.summary}</p>

                {report.findings.length > 0 && (
                  <div className="flex flex-col gap-1">
                    <h3 className="font-medium text-slate-800">Achados</h3>
                    <ul className="flex flex-col gap-1">
                      {[...report.findings]
                        .sort((a, b) => SEVERITY_ORDER.indexOf(a.severity) - SEVERITY_ORDER.indexOf(b.severity))
                        .map((finding, i) => (
                          <li key={i} className="flex gap-2">
                            <Badge
                              variant={finding.severity === "critical" ? "destructive" : "secondary"}
                              className={cn("shrink-0", finding.severity === "warning" && "bg-amber-100 text-amber-900")}
                            >
                              {SEVERITY_LABELS[finding.severity]}
                            </Badge>
                            <span>
                              <code className="text-xs text-muted-foreground">{finding.kind}</code> {finding.detail}
                            </span>
                          </li>
                        ))}
                    </ul>
                  </div>
                )}

                {report.actions.length > 0 && (
                  <div className="flex flex-col gap-1">
                    <h3 className="font-medium text-slate-800">Ações</h3>
                    <ul className="flex flex-col gap-1">
                      {report.actions.map((action, i) => (
                        <li key={i}>
                          <code className="text-xs text-muted-foreground">{action.tool}</code> {action.description}
                          {action.proposal_id && (
                            <span className="text-xs text-muted-foreground"> (proposta, veja acima)</span>
                          )}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            </details>
          )
        })}
      </CardContent>
    </Card>
  )
}
