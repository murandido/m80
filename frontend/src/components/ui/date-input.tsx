"use client"

import { useState } from "react"
import { CalendarDays } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  applyDateMask,
  isCompleteDateMask,
  isoDateToMasked,
  maskedDateToISO,
} from "@/lib/date-mask"
import { formatYmd, parseYmd } from "@/lib/date-range"
import { cn } from "@/lib/utils"

interface DateInputProps {
  // The date as the API speaks it, YYYY-MM-DD. Empty string means no date.
  value: string
  onChange: (value: string) => void
  // Bounds, in the same shape. The calendar refuses the days outside them and a typed date
  // beyond one is pulled back to it, which is what the native input's min/max did.
  min?: string
  max?: string
  // The calendar button. On by default; turned off inside the filter popovers, which
  // already show a calendar right above these fields.
  picker?: boolean
  id?: string
  disabled?: boolean
  className?: string
  "aria-invalid"?: boolean
}

// A date typed as DD/MM/AAAA, whatever language the computer is in.
//
// It replaces <input type="date">, which renders in the operating system's locale: the same
// panel showed DD/MM in its tables and MM/DD in its inputs depending on whose machine it
// was, with nothing saying which was which. The mask is the one the issuances grid has
// always used (src/lib/date-mask.ts), so the two now agree.
//
// The value in and out is always ISO, so no caller had to change how it stores a date.
export function DateInput({
  value,
  onChange,
  min,
  max,
  picker = true,
  id,
  disabled,
  className,
  "aria-invalid": ariaInvalid,
}: DateInputProps) {
  const [text, setText] = useState(() => (value ? isoDateToMasked(value) : ""))
  const [open, setOpen] = useState(false)
  // The last value this field saw from the outside - what it emitted, or what a parent
  // set. A parent echoing back what was just sent must not reset what is being typed:
  // "03/0" would jump to "03/09/2026" mid-keystroke.
  //
  // Adjusted during render rather than in an effect, which is React's own answer for a
  // field that has to follow a prop: the re-render happens before the DOM is touched, so
  // the input never paints the stale date.
  const [lastSeen, setLastSeen] = useState(value)
  if (value !== lastSeen) {
    setLastSeen(value)
    setText(value ? isoDateToMasked(value) : "")
  }

  function emit(iso: string) {
    setLastSeen(iso)
    onChange(iso)
  }

  // Pulls a date back inside min/max rather than refusing it: the bounds exist because the
  // caller knows the range, and the nearest day inside it is what the operator meant.
  function clamp(iso: string): string {
    if (min && iso < min) return min
    if (max && iso > max) return max
    return iso
  }

  function handleTyped(raw: string) {
    // A pasted ISO date is taken as it is - copying a date out of another screen and
    // dropping it here should work.
    if (/^\d{4}-\d{2}-\d{2}$/.test(raw.trim())) {
      const iso = clamp(raw.trim())
      setText(isoDateToMasked(iso))
      emit(iso)
      return
    }

    const masked = applyDateMask(raw)
    setText(masked)

    if (masked === "") {
      emit("")
      return
    }
    if (!isCompleteDateMask(masked)) return

    const iso = clamp(maskedDateToISO(masked))
    // The clamp may have moved it, so the text follows what was actually committed.
    if (iso !== maskedDateToISO(masked)) setText(isoDateToMasked(iso))
    emit(iso)
  }

  // A half-typed date is not a value: on the way out the field goes back to what is stored,
  // rather than leaving "03/0" on screen pretending to be something.
  function handleBlur() {
    if (text !== "" && !isCompleteDateMask(text)) {
      setText(value ? isoDateToMasked(value) : "")
    }
  }

  const selected = value ? parseYmd(value) : undefined

  return (
    <div className={cn("relative", className)}>
      <Input
        id={id}
        value={text}
        inputMode="numeric"
        placeholder="DD/MM/AAAA"
        disabled={disabled}
        aria-invalid={ariaInvalid}
        onChange={(event) => handleTyped(event.target.value)}
        onBlur={handleBlur}
        className={cn(picker && "pr-9")}
      />
      {picker && (
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              disabled={disabled}
              aria-label="Escolher no calendário"
              className="absolute right-0 top-0 h-full w-9 text-muted-foreground hover:bg-transparent"
            >
              <CalendarDays className="h-4 w-4" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-0" align="end">
            <Calendar
              mode="single"
              autoFocus
              selected={selected}
              defaultMonth={selected}
              disabled={[
                ...(min ? [{ before: parseYmd(min)! }] : []),
                ...(max ? [{ after: parseYmd(max)! }] : []),
              ]}
              onSelect={(day) => {
                if (!day) return
                const iso = formatYmd(day)
                setText(isoDateToMasked(iso))
                emit(iso)
                setOpen(false)
              }}
            />
          </PopoverContent>
        </Popover>
      )}
    </div>
  )
}
