"use client"

import { useId } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
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

export function ListSortSelect({ order }: { order: ListOrder }) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const selectId = useId()

  function apply(next: string) {
    const params = new URLSearchParams(searchParams.toString())
    params.set("order", next)
    // Row 21 of the old order is not row 21 of the new one.
    params.delete("page")
    router.replace(`${pathname}?${params.toString()}`)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={selectId} className="text-xs text-muted-foreground">
        Ordenar por
      </Label>
      <Select value={order} onValueChange={apply}>
        <SelectTrigger id={selectId} className="w-40">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {LIST_ORDER_KEYS.map((key) => (
            <SelectItem key={key} value={key}>
              {ORDER_LABELS[key]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
