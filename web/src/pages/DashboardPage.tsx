import { Separator } from '@/components/ui/separator'
import UpstreamStatus from '@/components/UpstreamStatus'
import KeyTable from '@/components/KeyTable'

export default function DashboardPage() {
  return (
    <div className="max-w-5xl mx-auto p-8">
      <UpstreamStatus />
      <Separator className="my-8" />
      <KeyTable />
    </div>
  )
}
