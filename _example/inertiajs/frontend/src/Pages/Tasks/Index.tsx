import { Link, useForm } from '@inertiajs/react'
import type { FormEvent } from 'react'
import { Layout } from '../../components/Layout'
import type { Task } from '../../types'

type TasksIndexProps = {
  tasks: Task[]
}

export default function TasksIndex({ tasks }: TasksIndexProps) {
  const form = useForm({
    title: '',
    notes: '',
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    form.post('/tasks', {
      preserveScroll: true,
      onSuccess: () => form.reset(),
    })
  }

  return (
    <Layout title="Tasks" eyebrow="Inertia route">
      <div className="task-page">
        <section className="panel task-list">
          <div className="panel-header">
            <h2>Current tasks</h2>
            <span>{tasks.length}</span>
          </div>
          <ul className="tasks">
            {tasks.map((task) => (
              <li key={task.id}>
                <Link href={`/tasks/${task.id}`} className="task-row">
                  <span>
                    <strong>{task.title}</strong>
                    <small>{task.notes || 'No notes'}</small>
                  </span>
                  <span className={`status status-${task.status}`}>{task.status}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>

        <section className="panel">
          <div className="panel-header">
            <h2>Add task</h2>
          </div>
          <form onSubmit={submit} className="task-form">
            <label>
              <span>Title</span>
              <input
                name="title"
                value={form.data.title}
                onChange={(event) => form.setData('title', event.target.value)}
                autoComplete="off"
              />
            </label>
            <label>
              <span>Notes</span>
              <textarea
                name="notes"
                value={form.data.notes}
                onChange={(event) => form.setData('notes', event.target.value)}
                rows={4}
              />
            </label>
            <button type="submit" disabled={form.processing}>
              {form.processing ? 'Creating...' : 'Create task'}
            </button>
          </form>
        </section>
      </div>
    </Layout>
  )
}
