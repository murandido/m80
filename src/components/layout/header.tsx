import { LogoSVG } from "@/components/icons/logo"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Lightbulb, Settings } from "lucide-react"

export function Header() {
  return (
    <header className="bg-slate-700">
      <div className="mx-12 flex items-center justify-between py-6 sm:mx-26">
        <div>
          <LogoSVG className="h-6 w-15 fill-white" />
        </div>
        <div className="lg:hidden">
          <Tabs defaultValue="management">
            <TabsList className="m-1">
              <TabsTrigger className="px-3 py-2" value="management">
                <Lightbulb className="size-6" />
              </TabsTrigger>
              <TabsTrigger className="px-3 py-2" value="settings">
                <Settings className="size-6" />
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </div>
    </header>
  )
}
