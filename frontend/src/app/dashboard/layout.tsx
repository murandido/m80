"use client"

import { Header } from "@/components/layout/header"
import { useState } from "react"
import { DesktopSidebar, MobileSidebar } from "@/components/layout/sidebar"

export default function DashboardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const [isSidebarOpen, setIsSidebarOpen] = useState(false)

  return (
    <>
      <Header setIsSidebarOpen={setIsSidebarOpen} />
      <main className="flex flex-1 overflow-hidden">
        <div
          className={`flex flex-1 transition-transform duration-500 ease-in-out ${isSidebarOpen && "-translate-x-full"}`}
        >
          <div className="w-screen shrink-0 lg:w-[75vw] lg:shrink">
            {children}
          </div>
          <div className="w-screen shrink-0 lg:shrink">
            <MobileSidebar />
            <DesktopSidebar />
          </div>
        </div>
      </main>
    </>
  )
}
