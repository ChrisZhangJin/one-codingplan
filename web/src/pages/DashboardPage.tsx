import { Separator } from '@/components/ui/separator'

export default function DashboardPage() {
  return (
    <div className="max-w-5xl mx-auto p-8">
      <h2 className="text-xl font-semibold leading-[1.2]">Upstream Status</h2>
      <p className="text-sm text-muted-foreground mt-2">Loading...</p>

      <Separator className="my-8" />

      <h2 className="text-xl font-semibold leading-[1.2]">Access Keys</h2>
      <p className="text-sm text-muted-foreground mt-2">Loading...</p>
    </div>
  )
}
