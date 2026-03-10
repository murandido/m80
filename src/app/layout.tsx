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
    <html lang="pt">
      <body className={`antialiased`}>{children}</body>
    </html>
  )
}
