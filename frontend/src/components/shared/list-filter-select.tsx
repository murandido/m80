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

// Radix Select cannot hold an empty-string value, so "no filter" needs a sentinel
// that never reaches the URL.
const ALL = "__all__"

interface ListFilterSelectProps {
  /** The URL key this control owns, e.g. "complete" or "loyalty_program_id". */
  name: string
  label: string
  /** Wording for the unfiltered state, e.g. "Todos os programas". */
  allLabel: string
  options: { value: string; label: string }[]
  /** Current value, resolved by the page from the URL. */
  value: string
  className?: string
}

export function ListFilterSelect({
  name,
  label,
  allLabel,
  options,
  value,
  className,
}: ListFilterSelectProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const selectId = useId()

  function apply(next: string) {
    const params = new URLSearchParams(searchParams.toString())
    if (next === ALL) {
      params.delete(name)
    } else {
      params.set(name, next)
    }
    // Narrowing the set invalidates the current page number.
    params.delete("page")
    router.replace(`${pathname}?${params.toString()}`)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={selectId} className="text-xs text-muted-foreground">
        {label}
      </Label>
      <Select value={value || ALL} onValueChange={apply}>
        <SelectTrigger id={selectId} className={className ?? "w-44"}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{allLabel}</SelectItem>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
