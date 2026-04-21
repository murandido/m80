import { Button } from "@/components/ui/button"
import { Trash2, Menu } from "lucide-react"
import { User } from "@/schemas/user"

interface UserCardProps {
  user: User
}

export function UserCard({ user }: UserCardProps) {
  return (
    <div className="flex items-center justify-between rounded-md border border-slate-200 px-4 py-3">
      <span className="text-sm font-medium text-slate-800">{user.name}</span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="icon"
          className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
        <Button
          variant="outline"
          size="icon"
          className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-cyan-400 hover:bg-cyan-400 hover:text-white active:border-cyan-800 active:bg-cyan-800 active:text-white"
        >
          <Menu className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
