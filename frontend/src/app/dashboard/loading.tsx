import { Spinner } from "@/components/ui/spinner"

export default function DashboardLoading() {
  return (
    <div className="flex min-h-[50vh] items-center justify-center p-6 sm:p-8">
      <Spinner className="h-8 w-8 text-slate-400" />
    </div>
  )
}
