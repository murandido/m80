"use client"

import { useState } from "react"
import Color from "color"
import { Palette } from "lucide-react"
import {
  ColorPicker,
  ColorPickerEyeDropper,
  ColorPickerHue,
  ColorPickerSelection,
} from "@/components/kibo-ui/color-picker"
import { Button } from "@/components/ui/button"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"

const HEX_REGEX = /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/
const FALLBACK_COLOR = "#64748B"

interface ColorPickerFieldProps {
  id?: string
  value: string
  onChange: (hex: string) => void
  disabled?: boolean
}

export function ColorPickerField({ id, value, onChange, disabled }: ColorPickerFieldProps) {
  const [open, setOpen] = useState(false)
  const isValidHex = HEX_REGEX.test(value)
  const swatchColor = isValidHex ? value : FALLBACK_COLOR

  return (
    <div className="flex gap-2">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            className="h-9 w-9 shrink-0 border-slate-300 p-0"
            style={{ backgroundColor: swatchColor }}
            aria-label="Selecionar cor"
          >
            <Palette className="h-4 w-4 text-white mix-blend-difference" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-64 p-3" align="start">
          <ColorPicker
            defaultValue={swatchColor}
            onChange={(rgba) => {
              const [r, g, b] = rgba as [number, number, number, number]
              const hex = Color.rgb(r, g, b).hex()
              onChange(hex)
            }}
            className="gap-3"
          >
            <ColorPickerSelection className="h-32" />
            <div className="flex items-center gap-2">
              <ColorPickerEyeDropper />
              <ColorPickerHue className="flex-1" />
            </div>
          </ColorPicker>
        </PopoverContent>
      </Popover>
      <InputGroup className="bg-slate-200">
        <InputGroupInput
          id={id}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={FALLBACK_COLOR}
          disabled={disabled}
        />
        <InputGroupAddon align="inline-end">
          <Palette className="text-foreground h-4 w-4" />
        </InputGroupAddon>
      </InputGroup>
    </div>
  )
}
