import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { promoStatusLabel, promoStatusTone } from "@/lib/promo"
import type { PromoEnrollment } from "@/schemas/promo"

// What was attempted, for whom and how it went. This is the answer to "who is already
// enrolled" - the sites cannot be asked, since the campaign this was built against
// reports success for a CPF that is already in.
export function EnrollmentHistory({
  enrollments,
}: {
  enrollments: PromoEnrollment[]
}) {
  if (enrollments.length === 0) {
    return (
      <p className="text-sm text-slate-500">Nenhum cadastro registrado ainda.</p>
    )
  }

  return (
    <div className="overflow-x-auto rounded-md border border-slate-200">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Quando</TableHead>
            <TableHead>CPF</TableHead>
            <TableHead>Promoção</TableHead>
            <TableHead>Resultado</TableHead>
            <TableHead>Quem mandou</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {enrollments.map((enrollment) => (
            <TableRow key={enrollment.id}>
              <TableCell className="whitespace-nowrap text-xs text-slate-600">
                {new Date(enrollment.created_at).toLocaleString("pt-BR", {
                  timeZone: "America/Sao_Paulo",
                  dateStyle: "short",
                  timeStyle: "short",
                })}
              </TableCell>
              <TableCell className="text-sm text-slate-800">
                {enrollment.document_name || "—"}
              </TableCell>
              <TableCell className="max-w-xs">
                <span className="block truncate text-xs text-slate-600" title={enrollment.url}>
                  {enrollment.recipe_name || enrollment.url_key}
                </span>
              </TableCell>
              <TableCell>
                <span
                  className={`rounded px-2 py-0.5 text-xs ${promoStatusTone(enrollment.status)}`}
                  title={enrollment.message ?? undefined}
                >
                  {promoStatusLabel(enrollment.status)}
                </span>
              </TableCell>
              <TableCell className="text-xs text-slate-600">
                {enrollment.created_by || "Painel"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
