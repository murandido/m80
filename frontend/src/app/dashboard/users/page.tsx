import { Button } from "@/components/ui/button"
import { UserCog } from "lucide-react"
import { Separator } from "@/components/ui/separator"

export default function UsersPage() {
  return (
    <div>
      <div className="mx-12 my-16 sm:mx-24 sm:my-32 lg:mr-12">
        <div>
          <div className="mx-2 flex flex-col gap-2 sm:gap-6">
            <h1 className="text-lg font-semibold text-slate-800">
              Tabela de Usuários
            </h1>
            <Button className="h-full bg-slate-800">
              <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
                <UserCog />
                Cadastrar Usuário
              </div>
            </Button>
            <Separator className="my-2" />
          </div>
        </div>
      </div>
    </div>
  )
}
