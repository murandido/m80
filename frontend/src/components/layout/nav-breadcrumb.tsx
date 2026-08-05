"use client"

import { Fragment } from "react"
import { usePathname } from "next/navigation"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { dashboardHome, findNavLocation } from "@/lib/navigation"

// Segments that live below a nav link and therefore have no entry of their own.
// Anything not listed here is dropped rather than shown raw — the ids in
// /dashboard/issuances/<uuid>/ticket would be noise in a breadcrumb.
const nestedSegmentLabels: Record<string, string> = {
  suppliers: "Fornecedores",
  ticket: "Bilhete",
}

interface Crumb {
  label: string
  href?: string
}

function buildCrumbs(pathname: string): Crumb[] {
  const location = findNavLocation(pathname)
  if (!location) {
    return [{ label: dashboardHome.title }]
  }

  const crumbs: Crumb[] = [
    { label: dashboardHome.title, href: dashboardHome.url },
    // The section is context, not a destination — it has no page of its own.
    { label: location.section.label },
    { label: location.link.title, href: location.link.url },
  ]

  for (const segment of pathname.slice(location.link.url.length).split("/")) {
    const label = nestedSegmentLabels[segment]
    if (label) crumbs.push({ label })
  }

  return crumbs
}

export function NavBreadcrumb() {
  const pathname = usePathname()
  const crumbs = buildCrumbs(pathname)

  return (
    <Breadcrumb>
      <BreadcrumbList className="flex-nowrap">
        {crumbs.map((crumb, index) => {
          const isLast = index === crumbs.length - 1

          return (
            <Fragment key={`${crumb.label}-${index}`}>
              <BreadcrumbItem
                // Intermediate crumbs are the first to go on a narrow screen;
                // the page you are on always survives.
                className={isLast ? "min-w-0" : "hidden sm:flex"}
              >
                {isLast ? (
                  <BreadcrumbPage className="truncate">{crumb.label}</BreadcrumbPage>
                ) : crumb.href ? (
                  <BreadcrumbLink href={crumb.href}>{crumb.label}</BreadcrumbLink>
                ) : (
                  // A section is context, not a destination — BreadcrumbPage
                  // would wrongly mark it as the current page.
                  <span className="truncate">{crumb.label}</span>
                )}
              </BreadcrumbItem>
              {!isLast && <BreadcrumbSeparator className="hidden sm:block" />}
            </Fragment>
          )
        })}
      </BreadcrumbList>
    </Breadcrumb>
  )
}
