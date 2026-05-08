import { Link } from '@inertiajs/react'
import { Layout } from '../../components/Layout'
import type { Task } from '../../types'

type TaskShowProps = {
  task: Task
}

export default function TaskShow({ task }: TaskShowProps) {
  const createdAt = new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(task.createdAt))

  return (
    <Layout title={task.title} eyebrow={`Task #${task.id}`}>
      <section className="panel detail">
        <div className="detail-meta">
          <span className={`status status-${task.status}`}>{task.status}</span>
          <span>{createdAt}</span>
        </div>
        <p>{task.notes || 'No notes were provided for this task.'}</p>
        <Link href="/tasks" className="secondary-link">
          Back to tasks
        </Link>
      </section>
    </Layout>
  )
}
