## TODO App Example

This is a simple example of a TODO Task App using tanukirpc with gentypescript and React.

### Requirements

- Go 1.22 or later
- Bun 1.1.24 or later

### Installation

#### launch server

```bash
$ go run github.com/mackee/tanukirpc/cmd/tanukiup -dir ./
```

#### launch frontend

```bash
$ cd frontend
$ bun install
$ bun run dev
```

### Note

* This example use on memory DB, so all data will be lost when the server is restarted.
