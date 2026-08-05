import {
  Award,
  Bot,
  Building2,
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

export type NavIcon = React.ComponentType<{ className?: string }>

export interface NavLink {
  title: string
  url: string
  icon: NavIcon
}

export interface NavSection {
  label: string
  // Hides the whole section from regular users. It sits on the section rather
  // than on each link because the admin screens are exactly one section — the
  // backend enforces the same restriction on its routes.
  adminOnly?: boolean
  items: NavLink[]
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
      { title: "Vendas", url: "/dashboard/sales", icon: HandCoins },
      { title: "Clientes", url: "/dashboard/clients", icon: UserRound },
    ],
  },
  {
    label: "Cadastros",
    items: [
      { title: "Contas", url: "/dashboard/accounts", icon: Landmark },
      { title: "Pessoas", url: "/dashboard/people", icon: Users },
      { title: "Agências", url: "/dashboard/agencies", icon: Building2 },
      { title: "Programas de Fidelidade", url: "/dashboard/loyalty-programs", icon: Award },
      { title: "CIAs Aéreas", url: "/dashboard/airlines", icon: Plane },
    ],
  },
  {
    label: "Administração",
    adminOnly: true,
    items: [
      { title: "Usuários", url: "/dashboard/users", icon: IdCard },
      { title: "Aeroportos", url: "/dashboard/airports", icon: MapPin },
      { title: "Bot", url: "/dashboard/bot", icon: Bot },
    ],
  },
]

// Real destinations reached from inside a page rather than from the sidebar.
// They belong in the command palette — searching for a page that exists and
// getting "nothing found" is worse than not searching at all — but adding them
// to the nav tree would duplicate a link the page already offers.
export const extraDestinations: (NavLink & { adminOnly?: boolean })[] = [
  {
    title: "Fornecedores",
    url: "/dashboard/sales/suppliers",
    icon: Landmark,
    adminOnly: true,
  },
]

export function visibleSections(isAdmin: boolean): NavSection[] {
  return navSections.filter((section) => !section.adminOnly || isAdmin)
}

export function visibleExtraDestinations(isAdmin: boolean): NavLink[] {
  return extraDestinations.filter((item) => !item.adminOnly || isAdmin)
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
  return null
}
