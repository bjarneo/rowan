# Configuration

Most settings are in the Settings view. This page lists the files and the
keys in `config.toml`, for people who keep their configuration in version
control or set up Rowan with a script.

## Files

Rowan keeps its files in the config directory of your system:

| System | Directory |
|---|---|
| Linux | `~/.config/rowan` |
| macOS | `~/Library/Application Support/rowan` |
| Windows | `%AppData%\rowan` |

To use another directory, set `ROWAN_CONFIG_DIR`.

| File | Contents |
|---|---|
| `config.toml` | Settings, connections, agent policies and saved queries. No passwords |
| `state.json` | Query history, recently opened items and editor drafts |
| `activity.jsonl` | Every agent call, one JSON object per line |
| `mcp-token` | The bearer token for the HTTP endpoint, mode 0600 |
| `credentials.json` | Passwords, only when the system keychain is not available, mode 0600 |
| `sample.sqlite` | The sample database that the first launch writes |

## Environment variables

| Variable | Effect |
|---|---|
| `ROWAN_CONFIG_DIR` | Use this directory for all files |
| `ROWAN_CREDENTIALS=file` | Store passwords in `credentials.json`, not in the keychain |

## config.toml

Rowan writes the file when a setting changes. You can edit it while Rowan is
closed. A missing key gets its default value.

### app

```toml
[app]
theme = "system"      # system, light or dark
layout = "command"    # command or workspace
accent = "violet"     # violet, blue, green or teal
density = "balanced"  # balanced or compact
row_limit = 1000      # rows that a query in the app fetches
```

### safety

```toml
[safety]
relock_after = "15m"              # the default unlock time, or "relaunch"
confirm_production_unlock = true  # type the name to unlock a production connection
confirm_destructive = true        # confirm DROP, TRUNCATE, and DELETE or UPDATE without WHERE
statement_timeout = "60s"         # each statement stops after this time
```

### mcp

```toml
[mcp]
enabled = true
http_port = 7421
tools = ["list_connections", "list_tables", "describe_table", "query", "explain", "execute"]
blocked_clients = []
```

Remove a tool from `tools` to turn it off for all agents. A client in
`blocked_clients` gets an error on each call.

### connections

Each connection is one `[[connections]]` table. Its agent policy is the
`[connections.mcp]` table that follows it.

```toml
[[connections]]
id = "acme-prod"
name = "acme-prod"
engine = "postgres"   # postgres, mysql or sqlite
host = "db.internal"
port = 5432
database = "acme"
user = "readonly"
ssl_mode = "require"
color = "red"
production = true
password = "keychain:rowan/acme-prod"

[connections.ssh]
enabled = true
host = "bastion.example.com"
port = 22
user = "deploy"
key = "~/.ssh/id_ed25519"

[connections.mcp]
exposed = true
access = "read-only"   # read-only or read-write
approve_writes = true
row_limit = 500
tables = ["orders", "customers"]   # empty means all tables
```

For SQLite, set `file` and leave out the server keys:

```toml
[[connections]]
id = "sample"
name = "sample.sqlite"
engine = "sqlite"
file = "~/.config/rowan/sample.sqlite"
color = "green"
```

Do not write a password into `config.toml`. The `password` key holds a
reference to the keychain or to `credentials.json`. To set a password, use the
connection dialog.

### snippets

Saved queries are `[[snippets]]` tables:

```toml
[[snippets]]
id = "s1"
name = "Unpaid invoices"
sql = "SELECT number, order_id, issued_at FROM invoices WHERE paid_at IS NULL;"
```
