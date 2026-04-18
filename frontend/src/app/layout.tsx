import type { Metadata } from "next"
import "./globals.css"

export const metadata: Metadata = {
  title: "M-80 porra",
  description: "Construindo mona",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="pt" className="min-h-screen">
      <body className={`flex min-h-screen flex-col antialiased`}>
        {children}
      </body>
    </html>
  )
}
