# Changelog

This file lists the notable changes in each release of Rowan.

The format follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/).
Rowan uses [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-03

### Added

- Rowan connects to PostgreSQL, MySQL and SQLite.
- Rowan has two layouts. The command layout opens everything from one search
  surface with a live preview. The workspace layout shows connections as tabs,
  with the editor and the grid in the center.
- A connection can use an SSH tunnel through a bastion host. Rowan checks the
  host key against `~/.ssh/known_hosts` and uses `ssh-agent` or a key file.
- Every connection opens read-only. The database enforces the read-only mode.
- To write, you unlock a connection for 5, 15, 30 or 60 minutes, or until
  relaunch. A production connection asks you to type its name first.
- Destructive statements ask for confirmation. Examples are `DROP`,
  `TRUNCATE`, and `DELETE` or `UPDATE` without `WHERE`.
- The MCP server listens on `127.0.0.1:7421/mcp` and needs a bearer token.
  Clients that speak only stdio use `rowan mcp --stdio`.
- Agent writes wait in an approval queue. Each queued write shows the reason,
  the SQL and the rows that change.
- Each connection has an agent policy: shared or hidden, read-only or
  read-write, write approval, row limit and table allowlist.
- The SQL editor completes tables, columns, keywords and functions. The list
  depends on the position of the caret.
- The command layout has a search syntax. For example, `orders status:shipped`
  filters on `status`, and `orders 10442` filters on the primary key. A bare
  word such as `refunded` matches a column value, also in a PostgreSQL enum.
- The `list_tables` tool takes optional `schema` and `filter` arguments. It
  returns at most 200 tables and gives the table count of each schema.
- Rowan caps a result on the server. PostgreSQL runs the query inside an
  outer `LIMIT`, and MySQL gets a `sql_select_limit` hint. A sort over
  millions of rows returns the first rows in milliseconds.
- The grid, the workspace sidebar and the editor render only what is in view.
  A result with 120 columns, a schema with thousands of tables and a script
  with thousands of lines scroll at full frame rate.
- A paste of 4,000 characters or more goes into the editor at once. WebKitGTK
  takes about 10 seconds to insert 3,000 lines one at a time. `Ctrl+Z` undoes
  the paste.
- A canceled PostgreSQL query keeps its connection. The connection pool keeps
  all of its connections open, so the row inspector and the search preview do
  not open a connection for each lookup.
- The row inspector does not count referencing rows through a column without
  an index. A new selection cancels the lookups for the previous row.
- The search preview shows the rows without a wait for a slow match count.
- `scripts/perf/bench.mjs` measures the backend calls and the UI on the demo
  database, in Chromium or in WebKitGTK.
- Export writes a query result to a CSV, JSON or SQL file.
- Rowan stores passwords in the system keychain.
- `rowan --version` and `rowan version` print the version.
