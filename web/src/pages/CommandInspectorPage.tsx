import { useParams } from 'react-router-dom'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody } from '../components/ui/Card'
import { CommandInspector } from '../components/activity/CommandInspector'

export function CommandInspectorPage() {
  const { uuid = '' } = useParams()
  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Activity', to: '/activity' }, { label: 'MDM Commands', to: '/activity/commands' }, { label: 'Inspector' }]}
        title="Command inspector"
        description="Every step of this MDM command from queue to device response. Sensitive values are redacted by default."
      />
      <Card>
        <CardBody>
          <CommandInspector key={uuid} uuid={uuid} />
        </CardBody>
      </Card>
    </>
  )
}
