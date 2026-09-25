"use client"

import { useId } from "react"
import { useUrlQuery } from "@/hooks/use-url-query"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Label } from "@/components/ui/label"
import { LIST_ORDER_KEYS, type ListOrder } from "@/lib/list-params"

// Labels rather than "name_asc": the operator sorts by what they see, and "A–Z" is
// the same phrase whether the column is called name, code or login.
const ORDER_LABELS: Record<ListOrder, string> = {
  name_asc: "A – Z",
  name_desc: "Z – A",
  created_desc: "Mais recentes",
  created_asc: "Mais antigos",
}

interface ListSortSelectProps {
  order: string
  /** Override for a listing whose sort keys are not the shared four - airports
      sort by code as well as by name, so "A – Z" alone would not say which. */
  options?: readonly { value: string; label: string }[]
}

export function ListSortSelect({ order, options }: ListSortSelectProps) {
  const { params: urlParams, isPending, update } = useUrlQuery()
  const selectId = useId()
  const shownOrder = isPending ? (urlParams.get("order") ?? order) : order

  function apply(next: string) {
    update((params) => {
      params.set("order", next)
      // Row 21 of the old order is not row 21 of the new one.
      params.delete("page")
    })
  }

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={selectId} className="text-xs text-muted-foreground">
        Ordenar por
      </Label>
      <Select value={shownOrder} onValueChange={apply}>
        <SelectTrigger id={selectId} className="w-44">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(
            options ??
            LIST_ORDER_KEYS.map((key) => ({ value: key, label: ORDER_LABELS[key] }))
          ).map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
