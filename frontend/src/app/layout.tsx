import type { Metadata } from "next"
import { Geist, Geist_Mono } from "next/font/google"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import "./globals.css"

// globals.css has always referenced --font-geist-sans/--font-geist-mono, but
// nothing ever defined them — the whole app fell back to the browser default.
const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
})

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
})

export const metadata: Metadata = {
  title: {
    default: "M-80",
    template: "%s · M-80",
  },
  description: "Painel de gestão de emissões, contas e programas de fidelidade.",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="pt-BR" className="min-h-screen">
      <body
        className={`${geistSans.variable} ${geistMono.variable} flex min-h-screen flex-col antialiased`}
      >
        {/* At the root rather than inside the dashboard: ui/tooltip.tsx is a bare
            Radix Root with no auto-provider, so every tooltip outside the sidebar
            tree used to fail silently. */}
        <TooltipProvider>{children}</TooltipProvider>
        <Toaster richColors position="top-right" />
      </body>
    </html>
  )
}
