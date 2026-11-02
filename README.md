# Rowan

Rowan is a desktop database client for PostgreSQL, MySQL and SQLite. Every
connection opens read-only. AI agents connect over MCP, and their writes wait
in an approval queue until you approve or deny them.

![Rowan in the workspace layout, with a query on a PostgreSQL table of 6 million rows](site/images/workspace-dark.png)

[![The 45-second promo video. Click to play it.](site/media/rowan-promo.jpg)](site/media/rowan-promo.mp4)

## What you can do

- **Browse production without risk.** The database refuses writes on the
  read pool. To write, you unlock a connection for a fixed time.
- **Let agents read, and approve each write.** Claude Code, Claude Desktop
  and Cursor read through MCP. Each write shows the reason, the SQL and the rows that
  change, and runs only after you approve it.
- **Write SQL with schema-aware completion.** Tables after `FROM`, columns
  for each alias, keywords in the case that you type.
- **Find rows without SQL.** Type `orders refunded` or
  `customers email~acme` and see the rows while you type.
- **Choose a layout.** One search surface, or a full workspace with tabs, a
  grid, a row inspector and a live agent feed.

| | |
|---|---|
| ![Completion after an alias](site/images/completion.png) | ![Agent writes in the approval queue](site/images/approval.png) |
| ![A write on a read-only connection](site/images/locked.png) | ![The command layout with a search preview](site/images/command-light.png) |

## Install

Download the build for your system from the [releases page](../../releases/latest):

- Linux: `rowan-<version>-linux-amd64.tar.gz` or `-linux-arm64.tar.gz`. Needs
  GTK 3 and WebKitGTK 4.1.
- macOS: `rowan-<version>-macos-universal.zip`
- Windows: `rowan-<version>-windows-amd64-installer.exe`

[Getting started](docs/getting-started.md) has the steps for each system.

To build from source, install Go, Node 20 and the Wails v2 CLI, then run:

```sh
wails build -tags webkit2_41
./build/bin/rowan
```

On macOS and Windows, leave out `-tags webkit2_41`.

## Connect an agent

Rowan serves MCP on `127.0.0.1:7421/mcp` while it runs. For Claude Code:

```sh
claude mcp add rowan -- /path/to/rowan mcp --stdio
```

The MCP server view in Rowan has the snippet for Claude Desktop, Cursor and
plain HTTP. A new connection is hidden from agents until you share it. See
[Agents and MCP](docs/agents.md).

## Safety model

- **Read-only at the database level.** The read pool sets
  `default_transaction_read_only` on PostgreSQL, `transaction_read_only` on
  MySQL and `query_only` on SQLite.
- **Writes need an unlock.** Unlock for 5, 15, 30 or 60 minutes, or until
  relaunch. Rowan then relocks the connection and closes its write pool.
- **Production connections** ask you to type the connection name before they
  unlock. Agent writes on them always wait for approval.
- **Destructive statements**, such as `DROP`, `TRUNCATE`, and `DELETE` or
  `UPDATE` without `WHERE`, ask for confirmation.
- **Statements that lift read-only mode** count as writes, such as
  `SET default_transaction_read_only = off`.

[Safety](docs/safety.md) has the details.

## Docs

- [Getting started](docs/getting-started.md)
- [Querying](docs/querying.md): search, the editor, completion and results
- [Safety](docs/safety.md)
- [Agents and MCP](docs/agents.md)
- [Configuration](docs/configuration.md)
- [Development](docs/development.md): build, tests and the demo database
- [Releasing](docs/releasing.md)
- [Changelog](CHANGELOG.md)

## Development

```sh
(cd frontend && npm ci && npm run build)
go test -tags webkit2_41 ./...
(cd frontend && npm test && npm run check)
```

To test against a PostgreSQL database with 46 million rows, run
`scripts/demo-pg/start.sh`. [Development](docs/development.md) shows the
engine tests, the headless UI tests and the scripts for the screenshots and
the promo video.

## Not built yet

The design explorations show these parts, but Rowan does not have them yet:

- the ER diagram
- the import view
- inline cell editing
- query notebooks with run diffs
