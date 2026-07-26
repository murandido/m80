"use client"

import { useState } from "react"
import { BarChart3, TableIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface ChartCardRow {
  key: string
  // Pre-formatted cells. The first is the label; the rest are values, right-aligned.
  cells: string[]
}

interface ChartCardProps {
  title: string
  description?: string
  children: React.ReactNode
  tableHeaders: string[]
  tableRows: ChartCardRow[]
  isEmpty?: boolean
}

// Every chart ships with a table twin. A tooltip enhances, it never gates: the same numbers
// have to be reachable without hovering - by keyboard, by screen reader, and by anyone who
// cannot separate two hues. It is also what licenses the lighter palette slots, whose fill
// sits below 3:1 against the card.
export function ChartCard({
  title,
  description,
  children,
  tableHeaders,
  tableRows,
  isEmpty = false,
}: ChartCardProps) {
  const [showTable, setShowTable] = useState(false)

  return (
    <Card className="shadow-none">
      <CardHeader>
        <CardTitle className="text-sm font-semibold text-slate-800">{title}</CardTitle>
        {description ? (
          <CardDescription className="text-xs">{description}</CardDescription>
        ) : null}
        <CardAction>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="text-slate-500"
            aria-pressed={showTable}
            onClick={() => setShowTable((current) => !current)}
          >
            {showTable ? (
              <>
                <BarChart3 className="h-4 w-4" />
                Ver gráfico
              </>
            ) : (
              <>
                <TableIcon className="h-4 w-4" />
                Ver tabela
              </>
            )}
          </Button>
        </CardAction>
      </CardHeader>

      <CardContent>
        {isEmpty ? (
          <p className="py-12 text-center text-sm text-slate-500">
            Nenhuma emissão no período selecionado.
          </p>
        ) : showTable ? (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  {tableHeaders.map((header, index) => (
                    <TableHead
                      key={header}
                      className={index === 0 ? undefined : "text-right"}
                    >
                      {header}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {tableRows.map((row) => (
                  <TableRow key={row.key}>
                    {row.cells.map((cell, index) => (
                      <TableCell
                        key={index}
                        className={
                          index === 0 ? undefined : "text-right tabular-nums"
                        }
                      >
                        {cell}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ) : (
          children
        )}
      </CardContent>
    </Card>
  )
}
