// Types that mirror the JSON from the Go backend.

export interface Policy {
  exposed: boolean
  access: 'read-only' | 'read-write'
  approveWrites: boolean
  rowLimit: number
  tables: string[] | null
}

export interface Connection {
  id: string
  name: string
  engine: 'postgres' | 'mysql' | 'sqlite'
  host: string
  port: number
  database: string
  user: string
  file: string
  sslMode: string
  sslRootCert: string
  color: string
  production: boolean
  hasPassword: boolean
  ssh: { enabled: boolean; host: string; port: number; user: string; key: string }
  mcp: Policy
}

export interface Snippet { id: string; name: string; sql: string }

export interface AppSettings {
  theme: 'system' | 'light' | 'dark'
  layout: 'command' | 'workspace'
  accent: 'violet' | 'blue' | 'green' | 'teal'
  density: 'compact' | 'balanced'
  rowLimit: number
}

export interface Safety {
  relockAfter: string
  confirmProductionUnlock: boolean
  confirmDestructive: boolean
  statementTimeout: string
}

export interface McpSettings {
  enabled: boolean
  httpPort: number
  tools: string[]
  blockedClients: string[] | null
}

export interface Config {
  app: AppSettings
  safety: Safety
  mcp: McpSettings
  connections: Connection[] | null
  snippets: Snippet[] | null
}

export interface HistoryItem { id: string; sql: string; conn: string; at: number; rows: number; ms: number; status: string }

export interface Recent {
  kind: 'table' | 'query' | 'view'
  conn: string
  schema?: string
  table?: string
  title: string
  sql?: string
  view?: string
  at: number
}

export interface State {
  history: HistoryItem[] | null
  recents: Recent[] | null
  drafts: Record<string, string> | null
  lastConn: string
}

export interface Change { column: string; from: string; to: string }

export interface QueueItem {
  id: string
  at: number
  client: string
  conn: string
  reason: string
  sql: string
  impact: string
  changes: Change[] | null
}

export interface LogEntry {
  id: string
  at: number
  client: string
  tool: string
  conn: string
  sql: string
  rows: number
  ms: number
  status: 'allowed' | 'truncated' | 'approved' | 'denied' | 'blocked' | 'error'
  note: string
  reason?: string
}

export interface ClientInfo { name: string; transport: string; since: number; lastSeen: number; calls: number; connected: boolean; blocked: boolean }
export interface McpStatus { running: boolean; port: number; error: string; clients: ClientInfo[] | null }

export interface Bootstrap {
  config: Config
  state: State
  locks: Record<string, number>
  queue: QueueItem[] | null
  log: LogEntry[] | null
  mcp: McpStatus
  configPath: string
  platform: string
  firstRun: boolean
  notice: string
}

export interface Column { name: string; type: string; numeric: boolean }

export interface TableSummary { schema: string; name: string; kind: 'table' | 'view'; rows: number; estimate: boolean }
export interface SchemaInfo { version: string; groups: { name: string; tables: TableSummary[] }[] | null }
/** The columns of every table on a connection, for SQL completion. */
export interface ColumnSet {
  tables: { schema: string; name: string; columns: { name: string; type: string }[] | null }[] | null
  truncated: boolean
}

export interface Ref { schema: string; table: string; column: string; local: string; indexed?: boolean }
export interface ColumnInfo { name: string; type: string; nullable: boolean; default: string; pk: boolean; unique: boolean; fk: Ref | null; numeric: boolean }
export interface TableInfo {
  schema: string
  name: string
  kind: string
  columns: ColumnInfo[]
  indexes: { name: string; columns: string; unique: boolean; primary: boolean }[] | null
  referencedBy: Ref[] | null
}

export type Cell = string | null

export interface RunResult {
  id: string
  columns: Column[] | null
  rows: Cell[][] | null
  truncated: boolean
  affected: number
  message: string
  ms: number
  statements: number
  write: boolean
  error: string
  locked: boolean
  confirm: string
  source: { schema: string; table: string } | null
}

export interface Filter { column: string; op: string; value: string; source: string; guess: boolean }
export interface Interp { filters: Filter[] | null; unresolved: string[] | null; note: string; canonical: string }

export interface PreviewResult {
  schema: string
  table: string
  columns: Column[] | null
  rows: Cell[][] | null
  interp: Interp
  matched: number
  matchedCapped: boolean
  total: number
  totalEstimate: boolean
  sql: string
  where: string
  error: string
  canceled: boolean
}

export interface Link { column: string; table: string; schema: string; label: string; count: number; capped: boolean; sql: string }
export interface Inspection { fks: Record<string, Link>; refBy: Link[] | null }
