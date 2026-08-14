import {
  Award,
  Bot,
  Building2,
  FileUp,
  HandCoins,
  IdCard,
  Landmark,
  LayoutDashboard,
  MapPin,
  Plane,
  Ticket,
  UserRound,
  Users,
} from "lucide-react"
import type { User } from "@/schemas/user"

export type NavIcon = React.ComponentType<{ className?: string }>

export type Role = User["role"]

export interface NavLink {
  title: string
  url: string
  icon: NavIcon
  // Which roles may reach this page. Omitted means everyone. dev is never listed:
  // it passes every check, mirroring RequireRole on the backend, so a page cannot
  // be hidden from dev by forgetting to name it here.
  roles?: Role[]
}

export interface NavSection {
  label: string
  items: NavLink[]
}

// canAccess is the single question the whole panel asks about permissions: the sidebar
// filter, the command palette and the components that show or hide a control all go
// through it, so "admin or dev" is spelled out in exactly one place.
export function canAccess(role: Role | undefined, roles?: Role[]): boolean {
  if (role === "dev") return true
  if (!roles) return true
  return role !== undefined && roles.includes(role)
}

// Add a new dashboard page by adding a link to the section it belongs to — the
// sidebar, the breadcrumb and the command palette all read from here, so no
// other file needs to change.
export const dashboardHome: NavLink = {
  title: "Dashboard",
  url: "/dashboard",
  icon: LayoutDashboard,
}

export const navSections: NavSection[] = [
  {
    label: "Operação",
    items: [
      { title: "Emissões", url: "/dashboard/issuances", icon: Ticket },
      { title: "Controle de pagamentos", url: "/dashboard/sales", icon: HandCoins },
    ],
  },
  {
    label: "Gestão",
    // Deliberately mixed: Contas and Clientes are everyday operator screens, while
    // Agências and Usuários are not. This is why permission moved from the section
    // onto each link.
    items: [
      { title: "Agências", url: "/dashboard/agencies", icon: Building2, roles: ["admin"] },
      { title: "Contas", url: "/dashboard/accounts", icon: Landmark },
      { title: "Clientes", url: "/dashboard/clients", icon: UserRound },
      { title: "Usuários do M80", url: "/dashboard/users", icon: IdCard, roles: ["admin"] },
      // Temporary: the bridge off the hand-kept spreadsheet. Remove this line with the
      // rest of the importer (see backend/internal/legacyimport).
      { title: "Importar planilha", url: "/dashboard/import", icon: FileUp, roles: ["admin"] },
    ],
  },
  {
    label: "Dev Mode",
    // Every item is dev-only, so the section simply disappears for everyone else —
    // visibleSections drops a section once nothing in it is reachable.
    items: [
      { title: "CIAs Aéreas", url: "/dashboard/airlines", icon: Plane, roles: [] },
      {
        title: "Programas de Fidelidade",
        url: "/dashboard/loyalty-programs",
        icon: Award,
        roles: [],
      },
      { title: "Aeroportos", url: "/dashboard/airports", icon: MapPin, roles: [] },
      { title: "Bot", url: "/dashboard/bot", icon: Bot, roles: [] },
    ],
  },
]

// Real destinations reached from inside a page rather than from the sidebar.
// They belong in the command palette — searching for a page that exists and
// getting "nothing found" is worse than not searching at all — but adding them
// to the nav tree would duplicate a link the page already offers.
//
// sectionLabel is what the breadcrumb shows in place of a parent section. Proprietários
// needs it spelled out: unlike Fornecedores, its URL is not nested under the sidebar link
// it is reached from, so nothing can be inferred from the path.
export const extraDestinations: (NavLink & { sectionLabel: string })[] = [
  {
    title: "Fornecedores",
    url: "/dashboard/sales/suppliers",
    icon: Landmark,
    roles: ["admin"],
    sectionLabel: "Operação",
  },
  {
    title: "Proprietários",
    url: "/dashboard/people",
    icon: Users,
    sectionLabel: "Gestão",
  },
]

export function visibleSections(role: Role | undefined): NavSection[] {
  return navSections
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => canAccess(role, item.roles)),
    }))
    .filter((section) => section.items.length > 0)
}

export function visibleExtraDestinations(role: Role | undefined): NavLink[] {
  return extraDestinations.filter((item) => canAccess(role, item.roles))
}

export function isNavLinkActive(pathname: string, url: string): boolean {
  // "/dashboard" would otherwise prefix-match every page in the panel.
  return url === dashboardHome.url ? pathname === url : pathname.startsWith(url)
}

/** The section and link a pathname belongs to, for the breadcrumb. */
export function findNavLocation(
  pathname: string,
): { section: NavSection; link: NavLink } | null {
  for (const section of navSections) {
    // Longest URL first so /dashboard/sales/suppliers can never be claimed by a
    // shorter sibling that happens to be a prefix of it.
    const link = [...section.items]
      .sort((a, b) => b.url.length - a.url.length)
      .find((item) => pathname.startsWith(item.url))
    if (link) return { section, link }
  }

  // Extra destinations are not in the tree, so without this a page like
  // /dashboard/people — which has no sidebar parent to prefix-match — would fall all the
  // way through and render a breadcrumb saying only "Dashboard".
  const extra = extraDestinations.find((item) => pathname.startsWith(item.url))
  if (extra) {
    return { section: { label: extra.sectionLabel, items: [] }, link: extra }
  }

  return null
}
