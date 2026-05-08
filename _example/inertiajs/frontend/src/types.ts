export type Task = {
  id: string
  title: string
  notes: string
  status: 'todo' | 'doing' | 'done'
  createdAt: string
}
