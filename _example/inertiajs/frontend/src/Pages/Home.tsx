import { Link } from '@inertiajs/react'
import { Layout } from '../components/Layout'

type HomeProps = {
  projectName: string
  taskCount: number
}

export default function Home({ projectName, taskCount }: HomeProps) {
  return (
    <Layout title={projectName}>
      <section className="summary-grid">
        <div className="metric">
          <span className="metric-label">Seed tasks</span>
          <strong>{taskCount}</strong>
        </div>
        <div className="copy-block">
          <p>
            This example serves the initial HTML shell from Go, then returns
            Inertia page JSON from the same route when the client sends
            <code>X-Inertia: true</code>.
          </p>
          <Link href="/tasks" className="primary-link">
            Open tasks
          </Link>
        </div>
      </section>
    </Layout>
  )
}
