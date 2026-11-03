# Development

This page shows how to build Rowan, run the tests, test the UI without a
window, and make the screenshots and the promo video.

## Layout of the code

| Path | Contents |
|---|---|
| `main.go`, `app.go`, `export.go` | The Wails app and the methods that the UI calls |
| `internal/db` | Connections, the read and write pools, introspection and the column set for completion |
| `internal/sqlx` | The SQL lexer and the statement classifier that decides what a write is |
| `internal/search` | The search syntax of the command layout |
| `internal/agent` | The MCP server, the tools, the approval queue and the stdio bridge |
| `internal/config`, `internal/state` | `config.toml`, the keychain and `state.json` |
| `frontend/src` | The Svelte 5 UI. `lib/complete.ts` is the completion engine |
| `scripts/demo-pg` | A PostgreSQL demo database with 46 million rows |
| `scripts/promo` | The scripts that make the screenshots and the promo video |
| `scripts/perf` | The benchmark of the backend calls and the UI on the demo database |
| `site` | The GitHub Pages site |

## Build

You need Go at the version in `go.mod`, Node 20 or later, and the Wails v2
CLI. On Linux you also need GTK 3 and WebKitGTK 4.1 with their headers, such
as `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`.

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails build -tags webkit2_41
./build/bin/rowan
```

For live reload during development:

```sh
wails dev -tags webkit2_41
```

On macOS and Windows, leave out `-tags webkit2_41`.

## Tests

```sh
(cd frontend && npm ci && npm run build)
go test -tags webkit2_41 ./...
(cd frontend && npm test && npm run check)
```

`go test` needs `frontend/dist`, because `main.go` embeds it. `npm test` runs
the completion tests in Node. `npm run check` runs svelte-check.

The engine tests also run against PostgreSQL and MySQL when you give them a
port:

```sh
docker run -d --name rowan-pg -e POSTGRES_PASSWORD=rowanpw -e POSTGRES_DB=acme -p 127.0.0.1:55432:5432 postgres:17-alpine
docker run -d --name rowan-my -e MYSQL_ROOT_PASSWORD=rowanpw -e MYSQL_DATABASE=shop -p 127.0.0.1:53306:3306 mysql:8.4
ROWAN_TEST_PG_PORT=55432 ROWAN_TEST_MYSQL_PORT=53306 go test -tags webkit2_41 ./internal/db/
```

## The demo PostgreSQL database

`scripts/demo-pg` loads a database that tests Rowan at scale. It has about 46
million rows in 1,137 tables:

- `orders` with 6 million rows, `order_items` with 18 million and
  `customers` with 1 million
- `analytics.events`, a table with one partition per month and 12 million
  rows
- `legacy`, a schema with 1,100 tables and 22,000 columns, more than the
  completion column set holds
- `type_zoo` with `uuid`, `jsonb`, arrays, `bytea`, `inet`, `interval`,
  `tsvector` and `point` columns
- `wide_metrics` with 120 columns, `"Audit Log"` with names that need quotes,
  an enum type, a view and a materialized view

```sh
scripts/demo-pg/start.sh
```

The seed takes 2 to 5 minutes and about 6 GB of disk. Then add a connection:
host `127.0.0.1`, port `55433`, database `acme`, user `postgres` and password
`rowan`. To remove the database, run `docker rm -f rowan-demo-pg`.

## Test the UI without a window

The dev server serves the UI and the Go bindings over HTTP, so the UI runs in
headless Chromium. It is not part of release builds.

```sh
(cd frontend && npm run build)
go build -tags "devserve webkit2_41" -o /tmp/rowan-dev .
ROWAN_CONFIG_DIR=/tmp/rowan-cfg ROWAN_CREDENTIALS=file ROWAN_DIST=$PWD/frontend/dist /tmp/rowan-dev --serve 127.0.0.1:5199
node scripts/cdp.mjs steps.json
```

`steps.json` is a list of steps such as `{"nav": "http://127.0.0.1:5199/"}`,
`{"type": "orders refunded"}`, `{"key": "Enter"}` and `{"shot": "out.png"}`.

## Measure performance

`scripts/perf/bench.mjs` starts the dev server with the demo database. It
times the backend calls, then runs the UI steps in `scripts/perf/suite.js`
and prints one line per step. A UI step ends at the frame after the step is
done.

```sh
scripts/demo-pg/start.sh
ROWAN_DEMO_PG_PORT=55433 node scripts/perf/bench.mjs
ROWAN_DEMO_PG_PORT=55433 ENGINE=webkit node scripts/perf/bench.mjs
```

- `ENGINE=webkit` runs the UI steps in WebKitGTK, the engine of the Linux
  app. It needs PyGObject and WebKitGTK 4.1. The view is offscreen, so no
  window opens, and it renders without the GPU.
- `CPU_SLOWDOWN=4` runs Chromium on a CPU that is 4 times slower.
- `NO_BUILD=1` skips the frontend build.

Run the benchmark before and after a change to the grid, the sidebar, the
editor or the query path. A step that takes more than 16 ms misses a frame.

## Screenshots and the promo video

The scripts in `scripts/promo` build the dev server, start it with a fresh
config, and drive the real UI. They need Go, Node, Chromium and ffmpeg.

```sh
scripts/demo-pg/start.sh
ROWAN_DEMO_PG_PORT=55433 node scripts/promo/shots.mjs
ROWAN_DEMO_PG_PORT=55433 node scripts/promo/video.mjs
```

- `shots.mjs` writes the six PNG files in `site/images` at 2880x1800.
- `video.mjs` writes `site/media/rowan-promo.mp4` at 1920x1080 and 30 frames
  per second, and the poster frame `rowan-promo.jpg`.

Without `ROWAN_DEMO_PG_PORT`, both scripts use the SQLite sample. The video
script captures one frame at a time, so the result is the same on a slow or a
fast machine. To skip the frontend build, set `NO_BUILD=1`.

## Releases

See [Releasing](releasing.md).
