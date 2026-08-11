"use client"

import { createContext, useContext, useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { Search } from "lucide-react"
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import {
  dashboardHome,
  visibleExtraDestinations,
  visibleSections,
  type Role,
} from "@/lib/navigation"

interface CommandPaletteState {
  open: () => void
}

const CommandPaletteContext = createContext<CommandPaletteState | null>(null)

function useCommandPalette(): CommandPaletteState {
  const context = useContext(CommandPaletteContext)
  if (!context) {
    throw new Error("useCommandPalette must be used within a CommandPaletteProvider")
  }
  return context
}

export function CommandPaletteProvider({
  children,
  role,
}: {
  children: React.ReactNode
  role?: Role
}) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "k" && (event.metaKey || event.ctrlKey)) {
        event.preventDefault()
        setIsOpen((previous) => !previous)
      }
    }

    document.addEventListener("keydown", handleKeyDown)
    return () => document.removeEventListener("keydown", handleKeyDown)
  }, [])

  function go(url: string) {
    setIsOpen(false)
    router.push(url)
  }

  return (
    <CommandPaletteContext.Provider value={{ open: () => setIsOpen(true) }}>
      {children}
      <CommandDialog
        open={isOpen}
        onOpenChange={setIsOpen}
        title="Buscar página"
        description="Digite o nome de uma página para navegar até ela."
      >
        <CommandInput placeholder="Buscar página…" />
        <CommandList>
          <CommandEmpty>Nenhuma página encontrada.</CommandEmpty>
          <CommandGroup>
            <CommandItem
              value={dashboardHome.title}
              onSelect={() => go(dashboardHome.url)}
            >
              <dashboardHome.icon />
              <span>{dashboardHome.title}</span>
            </CommandItem>
          </CommandGroup>
          {visibleSections(role).map((section) => (
            <CommandGroup key={section.label} heading={section.label}>
              {section.items.map((link) => (
                <CommandItem
                  key={link.url}
                  value={`${link.title} ${section.label}`}
                  onSelect={() => go(link.url)}
                >
                  <link.icon />
                  <span>{link.title}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          ))}
          {visibleExtraDestinations(role).length > 0 && (
            <CommandGroup heading="Outras páginas">
              {visibleExtraDestinations(role).map((link) => (
                <CommandItem
                  key={link.url}
                  value={link.title}
                  onSelect={() => go(link.url)}
                >
                  <link.icon />
                  <span>{link.title}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
        </CommandList>
      </CommandDialog>
    </CommandPaletteContext.Provider>
  )
}

export function CommandPaletteTrigger() {
  const { open } = useCommandPalette()

  return (
    <button
      type="button"
      onClick={open}
      className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-sidebar-foreground/70 transition-colors group-data-[collapsible=icon]:hidden hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
    >
      <Search className="size-3.5" />
      <span className="sr-only">Buscar página</span>
      <kbd aria-hidden className="font-sans">
        ⌘K
      </kbd>
    </button>
  )
}
