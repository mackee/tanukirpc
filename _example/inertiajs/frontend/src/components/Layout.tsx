import { Link } from '@inertiajs/react'
import type { PropsWithChildren } from 'react'

type LayoutProps = PropsWithChildren<{
  title: string
  eyebrow?: string
}>

export function Layout({ children, eyebrow = 'tanukirpc example', title }: LayoutProps) {
  return (
    <div className="app-shell">
      <header className="topbar">
        <Link href="/" className="brand">
          tanukirpc inertia
        </Link>
        <nav className="nav">
          <Link href="/" className="nav-link">
            Home
          </Link>
          <Link href="/tasks" className="nav-link">
            Tasks
          </Link>
        </nav>
      </header>
      <main className="main">
        <div className="page-heading">
          <p className="eyebrow">{eyebrow}</p>
          <h1>{title}</h1>
        </div>
        {children}
      </main>
    </div>
  )
}
