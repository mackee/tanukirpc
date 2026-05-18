import { type FormEvent, useState } from "react";
import "./App.css";
import { type ErrorResponse, isErrorResponse, newClient } from "./client";

// Empty baseURL → relative URLs. The frontend is served by Vite, but the
// browser hits tanukiup (port 8080) which routes /api/* to the Go server
// over UDS and forwards everything else to Vite via -catchall-target. The
// browser therefore only ever talks to one origin and no CORS is involved.
const client = newClient();

type FetchResult =
	| { kind: "idle" }
	| { kind: "loading" }
	| { kind: "ok"; user: { id: string; name: string } }
	| { kind: "err"; error: ErrorResponse };

function App() {
	const [id, setId] = useState("1");
	const [result, setResult] = useState<FetchResult>({ kind: "idle" });

	const fetchUser = async (e: FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		setResult({ kind: "loading" });

		const response = await client.get("/api/users/{id}", {
			pathArgs: { id },
		});

		// The generated `isErrorResponse` is a type guard derived from the
		// project's WithErrorBody marshaler — branching here narrows
		// `response` to the typed ErrorResponse on the error path and to the
		// success shape on the happy path. No casts, no manual field
		// inspection.
		if (isErrorResponse(response)) {
			setResult({ kind: "err", error: response });
			return;
		}
		setResult({ kind: "ok", user: response.user });
	};

	return (
		<>
			<h1>Typed Error Body Demo</h1>
			<p>
				Fetch <code>/api/users/&#123;id&#125;</code> and let the
				gentypescript-generated <code>isErrorResponse</code> narrow the
				response.
			</p>
			<form onSubmit={fetchUser}>
				<div className="row">
					<label htmlFor="user-id">User id</label>
					<input
						id="user-id"
						type="text"
						value={id}
						onChange={(e) => setId(e.target.value)}
					/>
					<button type="submit">Fetch</button>
					<button type="button" onClick={() => setId("999")}>
						Try missing
					</button>
					<button type="button" onClick={() => setId("1")}>
						Try existing
					</button>
				</div>
			</form>
			<Result result={result} />
		</>
	);
}

function Result({ result }: { result: FetchResult }) {
	switch (result.kind) {
		case "idle":
			return null;
		case "loading":
			return <p>Loading…</p>;
		case "ok":
			return (
				<pre className="result ok">
					{`200 OK
user.id   = ${result.user.id}
user.name = ${result.user.name}`}
				</pre>
			);
		case "err":
			return (
				<pre className="result err">
					{`${result.error.status} ${codeLine(result.error.code)}
message = ${result.error.message}`}
				</pre>
			);
	}
}

function codeLine(code: string | undefined): string {
	return code === undefined ? "" : code;
}

export default App;
