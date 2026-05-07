## Inertia.js Example

This example shows a small task app using tanukirpc with the opt-in Inertia.js codec.

### Requirements

- Go 1.25 or later
- Bun

### Install frontend dependencies

```bash
cd frontend
bun install
```

### Run

Start the Vite dev server:

```bash
cd frontend
bun run dev
```

Start the Go server in another terminal:

```bash
go run .
```

Open http://127.0.0.1:8080.

### What to look at

- `main.go` configures `codec.NewInertiajs` and returns `codec.Render(...)` from page handlers.
- `templates/app.html` is the root Inertia template served by Go.
- `frontend/src/main.tsx` boots the React Inertia app from the embedded page JSON.
