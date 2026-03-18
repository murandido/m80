import { NextRequest, NextResponse } from "next/server"

export function proxy(request: NextRequest) {
  const token = request.cookies.get("auth_token")?.value

  const { pathname } = request.nextUrl

  const isLoginPage = pathname === "/"
  const isDashboardRoute = pathname.startsWith("/dashboard")

  // logged and is login page
  if (token && isLoginPage) {
    return NextResponse.redirect(new URL("/dashboard", request.url))
  }

  // not logged and is dashboard route
  if (!token && isDashboardRoute) {
    return NextResponse.redirect(new URL("/", request.url))
  }

  return NextResponse.next()
}

export const config = {
  matcher: [
    /*
     * Execute the middleware on all routes EXCEPT those that begin with:
     * - api (Next's internal API routes, if any.)
     * - _next/static (static files, JS, CSS)
     * - _next/image (optimized image files)
     * - favicon.ico, sitemap.xml, robots.txt (system files)
     */
    "/((?!api|_next/static|_next/image|favicon.ico|sitemap.xml|robots.txt).*)",
  ],
}
