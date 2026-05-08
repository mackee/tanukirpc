import { Link } from '@inertiajs/react'
import { Layout } from '../components/Layout'

type ErrorProps = {
  status: number
  message: string
}

export default function ErrorPage({ message, status }: ErrorProps) {
  return (
    <Layout title={`${status}`} eyebrow="Error page">
      <section className="panel detail">
        <p>{message}</p>
        <Link href="/tasks" className="secondary-link">
          Back to tasks
        </Link>
      </section>
    </Layout>
  )
}
