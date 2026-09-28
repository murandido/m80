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
  const { params: urlParams, isPending, update } = useUrlQuery()
  const selectId = useId()
  // While a change is travelling, show what was picked rather than snapping back to
  // the server's value until the new page arrives.
  const shownValue = isPending ? (urlParams.get(name) ?? "") : value
  // Handed to SelectValue as children: on its own it fills the trigger only after
  // hydration, so the server HTML - the first paint - showed every filter empty.
  const shownLabel =
    options.find((option) => option.value === shownValue)?.label ?? allLabel

  function apply(next: string) {
    update((params) => {
      if (next === ALL) {
        params.delete(name)
      } else {
        params.set(name, next)
      }
      // Narrowing the set invalidates the current page number.
      params.delete("page")
    })
  }

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={selectId} className="text-xs text-muted-foreground">
        {label}
      </Label>
      <Select value={shownValue || ALL} onValueChange={apply}>
        <SelectTrigger id={selectId} className={className ?? "w-44"}>
          <SelectValue>{shownLabel}</SelectValue>
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
