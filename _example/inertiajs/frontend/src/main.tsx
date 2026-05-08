import { createInertiaApp } from '@inertiajs/react'
import { createRoot } from 'react-dom/client'
import type { ComponentType } from 'react'
import './styles.css'

type PageModule = {
  default: ComponentType<Record<string, unknown>>
}

const pages = import.meta.glob<PageModule>('./Pages/**/*.tsx', { eager: true })

const initialPageElement = document.querySelector<HTMLScriptElement>('script[data-page]')
const initialPage = initialPageElement?.textContent
  ? JSON.parse(initialPageElement.textContent)
  : undefined

createInertiaApp({
  page: initialPage,
  resolve: (name) => {
    const page = pages[`./Pages/${name}.tsx`]
    if (!page) {
      throw new Error(`Page not found: ${name}`)
    }
    return page.default
  },
  setup({ el, App, props }) {
    createRoot(el).render(<App {...props} />)
  },
})
