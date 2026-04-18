import { Card, CardContent } from "@/components/ui/card"
import { LogoSVG } from "@/components/icons/logo"
import { LoginForm } from "@/components/auth/LoginForm"

export default function Home() {
  return (
    <main className="flex h-screen w-screen items-center justify-center bg-white">
      <Card className="z-20 m-8 sm:m-38 lg:absolute lg:left-1/4 lg:m-0 lg:w-110 lg:-translate-x-1/2">
        <CardContent className="flex flex-col items-center justify-center">
          <LogoSVG className="my-20 h-11 w-26 fill-slate-700" />
          <div className="mx-6 flex flex-col items-center justify-center gap-6 text-slate-700">
            <LoginForm />
          </div>
        </CardContent>
      </Card>
      {/* blue bg */}
      <div className="absolute z-10 h-screen w-screen bg-slate-800 lg:right-0 lg:w-3/4"></div>
    </main>
  )
}
