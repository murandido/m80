import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatDateTime } from "@/lib/format"
import type { MCPCall } from "@/schemas/mcp"

// The trail of everything the agent did, including the writes it makes without asking. It is
// the only record of those: the issuance history does not log edits.
export function MCPActivityCard({ calls }: { calls: MCPCall[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Atividade</CardTitle>
        <CardDescription>As últimas 100 chamadas de ferramenta, com os argumentos enviados.</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Quando</TableHead>
                <TableHead>Ferramenta</TableHead>
                <TableHead>Token</TableHead>
                <TableHead>Resultado</TableHead>
                <TableHead>Argumentos</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {calls.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="py-6 text-center text-sm text-muted-foreground">
                    Nenhuma chamada ainda.
                  </TableCell>
                </TableRow>
              )}
              {calls.map((call) => (
                <TableRow key={call.id} className="align-top hover:bg-transparent">
                  <TableCell className="whitespace-nowrap text-muted-foreground">
                    {formatDateTime(call.created_at)}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{call.tool}</TableCell>
                  <TableCell>{call.token_name ?? "—"}</TableCell>
                  <TableCell>
                    {call.ok ? (
                      <Badge variant="secondary">OK · {call.duration_ms} ms</Badge>
                    ) : (
                      <span className="text-xs text-destructive">{call.error}</span>
                    )}
                  </TableCell>
                  <TableCell className="max-w-md">
                    <code className="text-xs break-all text-muted-foreground">
                      {call.args == null ? "" : JSON.stringify(call.args)}
                    </code>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}
