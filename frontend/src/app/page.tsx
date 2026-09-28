import type { Metadata } from "next"
import { Card, CardContent } from "@/components/ui/card"
import { LogoSVG } from "@/components/icons/logo"
import { LoginForm } from "@/components/auth/LoginForm"

export const metadata: Metadata = {
  title: "Entrar",
  description:
    "Acesse o painel M-80 com o telefone e a senha cadastrados por um administrador.",
}

export default function Home() {
  return (
    <main className="flex h-screen w-screen items-center justify-center bg-white">
      <Card className="z-20 m-8 sm:m-38 lg:absolute lg:left-1/4 lg:m-0 lg:w-110 lg:-translate-x-1/2">
        <CardContent className="flex flex-col items-center justify-center">
          <div className="my-20 flex flex-col items-center gap-3">
            <LogoSVG className="h-11 w-26 fill-slate-700" />
            <p className="text-center text-sm text-slate-500">
              Pré-visualização de teste (beta)
            </p>
          </div>
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
