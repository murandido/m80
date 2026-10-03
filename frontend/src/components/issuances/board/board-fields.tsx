"use client"

import {
  Children,
  cloneElement,
  isValidElement,
  useId,
  useRef,
  useState,
} from "react"
import { Loader2, Plus } from "lucide-react"
import { cn } from "@/lib/utils"
import { normalizeSearch } from "@/lib/search"
import {
  applyDateMask,
  isCompleteDateMask,
  isoDateToMasked,
  maskedDateToISO,
} from "@/lib/date-mask"
import { formatCurrencyBRL } from "@/lib/currency"
import { applyPaddedThousandsMask } from "@/lib/points"
import { DropdownPortal } from "@/components/issuances/cells/dropdown-portal"
import {
  formatNumberText,
  parseNumberText,
  type NumberMode,
} from "@/components/issuances/board/number-parsing"

// The form fields of the issuance panel. Every one of them saves on its own - on blur, on
// Enter, or on picking an option - because the panel autosaves like the grid did: there is
// no "Salvar" button, and an issuance becomes real the moment its required fields are in.
//
// Enter confirms and moves to the next field, the way Tab does. Typing an issuance is a run
// of short values, and the hand is already on the Enter key after each one.

// Where the panel's fields are looked up for Enter-to-advance. Only inputs marked as fields
// count, so the icon buttons in the header are not stops on the way.
const FIELD_SELECTOR = "input[data-field]:not(:disabled)"

export function focusSiblingField(from: HTMLElement, backwards = false) {
  const scope = from.closest("[data-issuance-form]")
  if (!scope) return
  const fields = Array.from(
    scope.querySelectorAll<HTMLInputElement>(FIELD_SELECTOR),
  )
  const index = fields.indexOf(from as HTMLInputElement)
  const next = fields[index + (backwards ? -1 : 1)]
  if (next) {
    next.focus()
    next.select()
  } else {
    from.blur()
  }
}

// Selects what a field was seeded with on focus, a frame later - the value only lands on the
// next render. Skipped when the operator has typed since: keys pressed right after Ctrl+Enter
// arrive before the frame, and selecting them would make the next key wipe them out.
function selectWhenSeeded(input: HTMLInputElement, seeded: string) {
  requestAnimationFrame(() => {
    if (input.value === seeded) input.select()
  })
}

export function focusField(scope: HTMLElement | null, name: string) {
  const field = scope?.querySelector<HTMLInputElement>(
    `input[data-field="${name}"]`,
  )
  field?.focus()
  field?.select()
}

// A coloured field: the program's or the airline's own colour (the design paints each pair
// in its brand), or the blue of a lookup in flight.
export interface FieldTint {
  color: string
  // The lookup state colours the label and the text too; a brand colour only the box.
  strong?: boolean
}

type JoinPosition = "single" | "first" | "middle" | "last"

const joinClasses: Record<JoinPosition, string> = {
  single: "rounded-lg",
  first: "rounded-l-lg rounded-r-none",
  middle: "rounded-none border-l-slate-200",
  last: "rounded-r-lg rounded-l-none border-l-slate-200",
}

// Several fields drawn as one box with dividers, as the design does for Programa|Cia, the
// two locators, Origem|Destino and the four values.
export function FieldGroup({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  const items = Children.toArray(children).filter(isValidElement)
  return (
    <div
      className={cn("grid gap-0", className)}
      style={{ gridTemplateColumns: `repeat(${items.length}, minmax(0, 1fr))` }}
    >
      {items.map((child, index) => {
        const join: JoinPosition =
          items.length === 1
            ? "single"
            : index === 0
              ? "first"
              : index === items.length - 1
                ? "last"
                : "middle"
        return cloneElement(
          child as React.ReactElement<{ join?: JoinPosition }>,
          { join },
        )
      })}
    </div>
  )
}

interface FieldShellProps {
  label: string
  inputId: string
  tint?: FieldTint | null
  join?: JoinPosition
  invalid?: boolean
  hint?: string | null
  className?: string
  children: (props: {
    className: string
    style: React.CSSProperties
  }) => React.ReactNode
  adornment?: React.ReactNode
}

function FieldShell({
  label,
  inputId,
  tint,
  join = "single",
  invalid,
  hint,
  className,
  children,
  adornment,
}: FieldShellProps) {
  const style: React.CSSProperties = tint
    ? {
        // ~6% of the colour over white: the tinted box of the design, readable under any brand.
        backgroundColor: `color-mix(in srgb, ${tint.color} ${tint.strong ? 9 : 6}%, white)`,
        borderColor: tint.strong
          ? `color-mix(in srgb, ${tint.color} 35%, white)`
          : tint.color,
        color: tint.strong ? tint.color : undefined,
      }
    : {}

  return (
    <div className={cn("flex min-w-0 flex-col gap-1.5", className)}>
      <label
        htmlFor={inputId}
        className="truncate text-sm text-slate-600 sm:text-base"
        style={tint?.strong ? { color: tint.color } : undefined}
      >
        {label}
      </label>
      <div className="relative">
        {children({
          className: cn(
            "h-11 w-full min-w-0 border border-transparent bg-white px-3 text-base text-slate-700 shadow-xs outline-none transition-colors placeholder:text-slate-400 focus-visible:z-10 focus-visible:border-sky-400 focus-visible:ring-[3px] focus-visible:ring-sky-200 read-only:cursor-default sm:px-4",
            joinClasses[join],
            join !== "single" && join !== "first" && "border-l-slate-200",
            invalid && "border-red-500 bg-red-50",
            adornment && "pr-9",
          ),
          style,
        })}
        {adornment && (
          <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2">
            {adornment}
          </span>
        )}
      </div>
      {hint && <p className="text-xs text-red-600">{hint}</p>}
    </div>
  )
}

interface BaseFieldProps {
  name: string
  label: string
  tint?: FieldTint | null
  join?: JoinPosition
  readOnly?: boolean
  invalid?: boolean
  hint?: string | null
  className?: string
  busy?: boolean
}

interface TextFieldProps extends BaseFieldProps {
  value: string | null
  onCommit: (value: string) => void
  uppercase?: boolean
  // Typing "/" here commits and hands over to another field - the grid's "programa/cia"
  // shorthand, kept for the operators who type it by reflex.
  onSlash?: () => void
  inputMode?: React.HTMLAttributes<HTMLInputElement>["inputMode"]
}

export function TextField({
  name,
  label,
  value,
  onCommit,
  uppercase,
  onSlash,
  inputMode,
  tint,
  join,
  readOnly,
  invalid,
  hint,
  className,
  busy,
}: TextFieldProps) {
  const id = useId()
  const [draft, setDraft] = useState<string | null>(null)
  const shown = draft ?? value ?? ""

  function commit() {
    if (draft === null) return
    const next = draft.trim()
    setDraft(null)
    if (next !== (value ?? "")) onCommit(next)
  }

  return (
    <FieldShell
      label={label}
      inputId={id}
      tint={tint}
      join={join}
      invalid={invalid}
      hint={hint}
      className={className}
      adornment={
        busy ? <Loader2 className="h-4 w-4 animate-spin text-sky-500" /> : null
      }
    >
      {(shell) => (
        <input
          id={id}
          data-field={name}
          {...shell}
          value={shown}
          placeholder="..."
          readOnly={readOnly}
          inputMode={inputMode}
          autoComplete="off"
          spellCheck={false}
          onFocus={(event) => event.currentTarget.select()}
          onChange={(event) => {
            const text = uppercase
              ? event.target.value.toUpperCase()
              : event.target.value
            if (onSlash && text.includes("/")) {
              setDraft(text.replace("/", ""))
              // Committed through the blur that moving focus causes.
              requestAnimationFrame(onSlash)
              return
            }
            setDraft(text)
          }}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault()
              focusSiblingField(event.currentTarget, event.shiftKey)
            } else if (event.key === "Escape" && draft !== null) {
              // First Escape drops the edit; the panel only sees the second.
              event.stopPropagation()
              setDraft(null)
            }
          }}
        />
      )}
    </FieldShell>
  )
}

interface NumberFieldProps extends BaseFieldProps {
  value: number | null
  mode: NumberMode
  onCommit: (value: number | null) => void
}

// Points in milheiros, money in reais; either may be typed as a sum. Shows the formatted
// value at rest and the editable one while focused, like the grid's number cell.
export function NumberField({
  name,
  label,
  value,
  mode,
  onCommit,
  tint,
  join,
  readOnly,
  invalid,
  hint,
  className,
}: NumberFieldProps) {
  const id = useId()
  const [draft, setDraft] = useState<string | null>(null)
  const display =
    value === null
      ? ""
      : mode === "currency"
        ? formatCurrencyBRL(value)
        : applyPaddedThousandsMask(String(value))

  function commit() {
    if (draft === null) return
    const text = draft.trim()
    setDraft(null)
    if (text === "") {
      if (value !== null) onCommit(null)
      return
    }
    const parsed = parseNumberText(text, mode)
    if (parsed !== null && parsed !== value) onCommit(parsed)
  }

  return (
    <FieldShell
      label={label}
      inputId={id}
      tint={tint}
      join={join}
      invalid={invalid}
      hint={hint}
      className={className}
    >
      {(shell) => (
        <input
          id={id}
          data-field={name}
          {...shell}
          className={cn(shell.className, "tabular-nums")}
          value={draft ?? display}
          placeholder="..."
          readOnly={readOnly}
          inputMode="decimal"
          autoComplete="off"
          title={
            mode === "points"
              ? "Em milheiros: 256 = 256.000; aceita conta, como 120+136"
              : undefined
          }
          onFocus={(event) => {
            if (readOnly) return
            const text = formatNumberText(value, mode)
            setDraft(text)
            selectWhenSeeded(event.currentTarget, text)
          }}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault()
              focusSiblingField(event.currentTarget, event.shiftKey)
            } else if (event.key === "Escape" && draft !== null) {
              event.stopPropagation()
              setDraft(formatNumberText(value, mode))
            }
          }}
        />
      )}
    </FieldShell>
  )
}

// A read-only box drawn like the others - the Total.
export function DisplayField({
  label,
  value,
  join,
  className,
}: {
  label: string
  value: string
  join?: JoinPosition
  className?: string
}) {
  const id = useId()
  return (
    <FieldShell label={label} inputId={id} join={join} className={className}>
      {(shell) => (
        <input
          id={id}
          {...shell}
          className={cn(shell.className, "tabular-nums")}
          value={value}
          placeholder="..."
          readOnly
          tabIndex={-1}
        />
      )}
    </FieldShell>
  )
}

interface DateFieldProps extends BaseFieldProps {
  value: string | null // ISO
  onCommit: (value: string | null) => void
  // Data da emissão can never be empty - an issuance always happened on some day.
  required?: boolean
}

export function DateField({
  name,
  label,
  value,
  onCommit,
  required,
  tint,
  join,
  readOnly,
  invalid,
  hint,
  className,
  busy,
}: DateFieldProps) {
  const id = useId()
  const [draft, setDraft] = useState<string | null>(null)
  const display = value ? isoDateToMasked(value) : ""

  function commit() {
    if (draft === null) return
    const text = draft.trim()
    setDraft(null)
    if (text === "") {
      if (!required && value !== null) onCommit(null)
      return
    }
    if (!isCompleteDateMask(text)) return
    const iso = maskedDateToISO(text)
    if (iso !== value) onCommit(iso)
  }

  return (
    <FieldShell
      label={label}
      inputId={id}
      tint={tint}
      join={join}
      invalid={invalid}
      hint={hint}
      className={className}
      adornment={
        busy ? <Loader2 className="h-4 w-4 animate-spin text-sky-500" /> : null
      }
    >
      {(shell) => (
        <input
          id={id}
          data-field={name}
          {...shell}
          className={cn(shell.className, "tabular-nums")}
          value={draft ?? display}
          placeholder="DD/MM/AAAA"
          readOnly={readOnly}
          inputMode="numeric"
          autoComplete="off"
          onFocus={(event) => event.currentTarget.select()}
          onChange={(event) => setDraft(applyDateMask(event.target.value))}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault()
              focusSiblingField(event.currentTarget, event.shiftKey)
            } else if (event.key === "Escape" && draft !== null) {
              event.stopPropagation()
              setDraft(null)
            }
          }}
        />
      )}
    </FieldShell>
  )
}

export interface ComboOption {
  key: string
  // What the field shows once picked.
  label: string
  // Shown beside the label in the list and searched too - a program's full name next to
  // its code, an airport's city.
  hint?: string
}

interface ComboFieldProps extends BaseFieldProps {
  options: ComboOption[] | ((query: string) => ComboOption[])
  selectedKey: string | null
  // What the field reads when nothing is focused. Defaults to the selected option's label;
  // an account being created shows the typed alias while its requests run.
  displayValue?: string
  onPick: (option: ComboOption) => void
  // Text that matches no option. Returning true means it was taken (an airport code typed
  // whole); otherwise the field goes back to its value.
  onFreeText?: (text: string) => boolean
  create?: {
    label: (typed: string) => string
    hint: (typed: string) => string
    disabledReason: string | null
    onCreate: (typed: string) => void
  }
  onSlash?: () => void
}

function filterOptions(options: ComboOption[], typed: string): ComboOption[] {
  const query = normalizeSearch(typed)
  if (query === "") return options
  return options
    .filter(
      (option) =>
        normalizeSearch(option.label).includes(query) ||
        (option.hint ? normalizeSearch(option.hint).includes(query) : false),
    )
    .sort((a, b) => {
      const aStarts = normalizeSearch(a.label).startsWith(query)
      const bStarts = normalizeSearch(b.label).startsWith(query)
      if (aStarts === bStarts) return 0
      return aStarts ? -1 : 1
    })
}

// A type-to-search picker. Focusing it lists everything; typing narrows; the arrows move the
// highlight; Enter picks it and moves on. Tab picks the highlight only after the operator
// typed or arrowed - otherwise tabbing through an untouched field would write whatever
// happened to be first in the list.
export function ComboField({
  name,
  label,
  options,
  selectedKey,
  displayValue,
  onPick,
  onFreeText,
  create,
  onSlash,
  tint,
  join,
  readOnly,
  invalid,
  hint,
  className,
  busy,
}: ComboFieldProps) {
  const id = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState<string | null>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  const [interacted, setInteracted] = useState(false)
  // Set when a pick already resolved this edit, so the blur that follows does not resolve
  // it a second time from the text.
  const resolvedRef = useRef(false)

  const resolve = (typed: string) =>
    typeof options === "function"
      ? options(typed)
      : filterOptions(options, typed)
  const allOptions = typeof options === "function" ? null : options
  const selectedLabel =
    displayValue ??
    allOptions?.find((option) => option.key === selectedKey)?.label ??
    ""

  const isEditing = query !== null
  const filtered = isEditing ? resolve(query) : []
  const exactMatch = isEditing
    ? filtered.find(
        (option) => normalizeSearch(option.label) === normalizeSearch(query),
      )
    : undefined
  const showCreate =
    create !== undefined &&
    isEditing &&
    query.trim() !== "" &&
    exactMatch === undefined
  const createSelectable = showCreate && create.disabledReason === null
  const itemCount = filtered.length + (createSelectable ? 1 : 0)
  const open = isEditing && (filtered.length > 0 || showCreate)

  function finish() {
    setQuery(null)
    setInteracted(false)
  }

  function pick(option: ComboOption) {
    resolvedRef.current = true
    finish()
    if (option.key !== selectedKey) onPick(option)
  }

  function pickCreate(typed: string) {
    resolvedRef.current = true
    finish()
    create?.onCreate(typed.trim())
  }

  // Leaving the field without picking: an exact match is taken, free text is offered to
  // the caller, and anything else goes back to what the field held.
  function resolveOnBlur() {
    if (resolvedRef.current || query === null) {
      resolvedRef.current = false
      finish()
      return
    }
    const typed = query.trim()
    if (
      typed === "" ||
      normalizeSearch(typed) === normalizeSearch(selectedLabel)
    ) {
      finish()
      return
    }
    if (exactMatch) {
      pick(exactMatch)
      resolvedRef.current = false
      return
    }
    onFreeText?.(typed)
    finish()
  }

  // Accepts what is highlighted; false when there was nothing to accept.
  function acceptHighlighted(): boolean {
    if (!open) return false
    if (createSelectable && activeIndex === filtered.length) {
      pickCreate(query ?? "")
      return true
    }
    const option = filtered[activeIndex]
    if (!option) return false
    pick(option)
    return true
  }

  function scrollHighlight(index: number) {
    requestAnimationFrame(() =>
      listRef.current?.children[index]?.scrollIntoView({ block: "nearest" }),
    )
  }

  return (
    <FieldShell
      label={label}
      inputId={id}
      tint={tint}
      join={join}
      invalid={invalid}
      hint={hint}
      className={className}
      adornment={
        busy ? <Loader2 className="h-4 w-4 animate-spin text-sky-500" /> : null
      }
    >
      {(shell) => (
        <>
          <input
            id={id}
            ref={inputRef}
            data-field={name}
            {...shell}
            value={isEditing ? query : selectedLabel}
            placeholder="..."
            readOnly={readOnly}
            autoComplete="off"
            spellCheck={false}
            role="combobox"
            aria-expanded={open}
            aria-controls={`${id}-list`}
            onFocus={(event) => {
              if (readOnly) return
              resolvedRef.current = false
              setQuery(selectedLabel)
              const options = resolve("")
              const selectedIndex = options.findIndex(
                (option) => option.key === selectedKey,
              )
              setActiveIndex(selectedIndex === -1 ? 0 : selectedIndex)
              setInteracted(false)
              selectWhenSeeded(event.currentTarget, selectedLabel)
            }}
            onChange={(event) => {
              const text = event.target.value
              if (onSlash && text.includes("/")) {
                const typed = text.replace("/", "")
                const match = resolve(typed)[0]
                if (match && typed.trim() !== "") pick(match)
                else finish()
                requestAnimationFrame(onSlash)
                return
              }
              setQuery(text)
              setActiveIndex(0)
              setInteracted(true)
            }}
            onBlur={resolveOnBlur}
            onKeyDown={(event) => {
              if (readOnly) return
              if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                event.preventDefault()
                if (!open) return
                const next =
                  event.key === "ArrowDown"
                    ? Math.min(activeIndex + 1, itemCount - 1)
                    : Math.max(activeIndex - 1, 0)
                setActiveIndex(next)
                setInteracted(true)
                scrollHighlight(next)
              } else if (event.key === "Enter") {
                event.preventDefault()
                if (interacted) acceptHighlighted()
                focusSiblingField(event.currentTarget, event.shiftKey)
              } else if (event.key === "Tab") {
                // Not prevented: the browser moves focus, and the pick lands first.
                if (interacted) acceptHighlighted()
              } else if (event.key === "Escape" && isEditing) {
                event.stopPropagation()
                resolvedRef.current = true
                finish()
                event.currentTarget.blur()
              }
            }}
          />
          {open && (
            <DropdownPortal anchorRef={inputRef}>
              <div ref={listRef} id={`${id}-list`} role="listbox">
                {filtered.map((option, index) => (
                  <button
                    key={option.key}
                    type="button"
                    role="option"
                    aria-selected={index === activeIndex}
                    tabIndex={-1}
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={() => {
                      pick(option)
                      if (inputRef.current) focusSiblingField(inputRef.current)
                    }}
                    className={cn(
                      "hover:bg-accent flex w-full items-baseline gap-2 px-3 py-2 text-left text-sm",
                      index === activeIndex && "bg-accent",
                    )}
                  >
                    <span className="shrink-0 font-medium">{option.label}</span>
                    {option.hint && (
                      <span className="text-muted-foreground truncate text-xs">
                        {option.hint}
                      </span>
                    )}
                  </button>
                ))}
                {showCreate &&
                  (create.disabledReason !== null ? (
                    <p className="text-muted-foreground border-t px-3 py-2 text-xs">
                      {create.disabledReason}
                    </p>
                  ) : (
                    <button
                      type="button"
                      tabIndex={-1}
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => pickCreate(query ?? "")}
                      className={cn(
                        "hover:bg-accent flex w-full items-start gap-1.5 border-t px-3 py-2 text-left",
                        activeIndex === filtered.length && "bg-accent",
                      )}
                    >
                      <Plus className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                      <span className="min-w-0">
                        <span className="block truncate text-sm">
                          {create.label(query ?? "")}
                        </span>
                        <span className="text-muted-foreground block text-xs">
                          {create.hint(query ?? "")}
                        </span>
                      </span>
                    </button>
                  ))}
              </div>
            </DropdownPortal>
          )}
        </>
      )}
    </FieldShell>
  )
}
