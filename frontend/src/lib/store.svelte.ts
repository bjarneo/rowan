import { api, on } from './api'
import { uid, plural, fmtN } from './format'
import { tableRef } from './sql'
import type { Col, Source } from './complete'
import type {
  Config, Connection, SchemaInfo, RunResult, QueueItem, LogEntry, McpStatus, HistoryItem, Recent,
  AppSettings, Safety, TableSummary,
} from './types'

/** Columns by "schema.table", for SQL completion. */
interface ColIndex {
  map: Map<string, Col[]>
  /** The backend capped the set. Missing tables load one at a time. */
  truncated: boolean
}

export type View =
  | { kind: 'home' }
  | { kind: 'table'; conn: string; schema: string; table: string; rest: string; sql: string }
  | { kind: 'query'; conn: string }
  | { kind: 'log'; q: string }
  | { kind: 'mcp' }
  | { kind: 'settings' }

export type Rail = 'tables' | 'mcp' | 'log' | 'settings'

/** The editor and result of one connection. */
export class Session {
  conn: string
  sql = $state('')
  selection = $state('')
  result = $state.raw<RunResult | null>(null)
  running = $state(false)
  selected = $state(-1)
  label = $state('')
  ranSQL = $state('')
  dirty = $state(false)
  private runId = ''
  private draftTimer = 0

  constructor(conn: string, sql: string) {
    this.conn = conn
    this.sql = sql
  }

  /** Writes the draft now, for example when the window closes. */
  flush() {
    clearTimeout(this.draftTimer)
    api.setDraft(this.conn, this.sql)
  }

  setSQL(sql: string, dirty = true) {
    this.sql = sql
    this.dirty = dirty
    clearTimeout(this.draftTimer)
    this.draftTimer = window.setTimeout(() => api.setDraft(this.conn, this.sql), 500)
  }

  async run(opts: { force?: boolean; sql?: string; label?: string } = {}) {
    const sql = (opts.sql ?? this.target()).trim()
    if (!sql || this.running) return
    this.running = true
    this.runId = uid('run')
    this.label = opts.label ?? ''
    const started = Date.now()
    let r: RunResult
    try {
      r = await api.run({ runId: this.runId, conn: this.conn, sql, limit: app.config.app.rowLimit, force: !!opts.force })
    } catch (e) {
      r = { error: String(e) } as RunResult
    }
    this.running = false
    this.ranSQL = sql
    this.result = r
    this.selected = r.rows && r.rows.length ? 0 : -1
    if (!r.confirm) {
      app.addHistory({ id: uid('h'), sql, conn: this.conn, at: started, rows: r.rows ? r.rows.length : r.affected, ms: r.ms, status: r.error ? (r.locked ? 'blocked' : 'error') : 'ok' })
    }
    if (r.write && !r.error && !r.confirm) app.loadSchema(this.conn, true)
  }

  /** The selected text when there is one, else the whole editor. */
  target() {
    const sel = this.selection.trim()
    return sel && this.sql.includes(sel) ? sel : this.sql
  }

  async explain() {
    const sql = this.target().trim()
    if (!sql || this.running) return
    this.running = true
    this.runId = uid('run')
    try {
      this.result = await api.explain({ runId: this.runId, conn: this.conn, sql, limit: 1000, force: false })
    } finally {
      this.running = false
    }
    this.selected = -1
    this.label = 'plan'
  }

  cancel() {
    if (this.running) api.cancel(this.runId)
  }
}

class Store {
  ready = $state(false)
  bootError = $state('')
  config = $state.raw<Config>(null as unknown as Config)
  configPath = ''
  platform = ''
  history = $state.raw<HistoryItem[]>([])
  recents = $state.raw<Recent[]>([])
  locks = $state.raw<Record<string, number>>({})
  queue = $state.raw<QueueItem[]>([])
  log = $state.raw<LogEntry[]>([])
  mcp = $state.raw<McpStatus>({ running: false, port: 7421, error: '', clients: [] })
  activeConn = $state('')
  schemas = $state.raw<Record<string, SchemaInfo>>({})
  schemaErr = $state.raw<Record<string, string>>({})
  schemaLoading = $state.raw<Record<string, boolean>>({})
  columns = $state.raw<Record<string, ColIndex>>({})

  view = $state.raw<View>({ kind: 'home' })
  rail = $state<Rail>('tables')
  palette = $state(false)
  queueOpen = $state(false)
  unlockReq = $state.raw<{ conn: string; then?: () => void } | null>(null)
  connDialog = $state.raw<{ id: string | null } | null>(null)
  toastMsg = $state<string | null>(null)
  now = $state(Date.now())
  systemDark = $state(false)

  private columnsLoading = new Set<string>()
  private sessions: Record<string, Session> = {}
  private drafts: Record<string, string> = {}
  private toastTimer = 0

  get conns(): Connection[] {
    return this.config?.connections ?? []
  }
  get active(): Connection | undefined {
    return this.conns.find((c) => c.id === this.activeConn)
  }
  get theme(): 'light' | 'dark' {
    const t = this.config?.app.theme ?? 'system'
    return t === 'system' ? (this.systemDark ? 'dark' : 'light') : t
  }
  get layout() {
    return this.config?.app.layout ?? 'command'
  }

  conn(id: string) {
    return this.conns.find((c) => c.id === id)
  }

  unlocked(id: string) {
    const u = this.locks[id]
    return u === -1 || (u !== undefined && u > this.now)
  }

  async init() {
    const mq = matchMedia('(prefers-color-scheme: dark)')
    this.systemDark = mq.matches
    mq.addEventListener('change', () => (this.systemDark = mq.matches))
    setInterval(() => (this.now = Date.now()), 1000)
    try {
      const b = await api.bootstrap()
      this.config = b.config
      this.configPath = b.configPath
      this.platform = b.platform
      this.history = b.state.history ?? []
      this.recents = b.state.recents ?? []
      this.drafts = b.state.drafts ?? {}
      this.locks = b.locks ?? {}
      this.queue = b.queue ?? []
      this.log = b.log ?? []
      this.mcp = b.mcp
      const last = b.state.lastConn
      this.activeConn = this.conns.some((c) => c.id === last) ? last : this.conns[0]?.id ?? ''
      if (b.notice) this.toast(b.notice, 6000)
    } catch (e) {
      this.bootError = String(e)
      return
    }
    window.addEventListener('beforeunload', () => Object.values(this.sessions).forEach((s) => s.flush()))
    on('queue', (q) => (this.queue = q ?? []))
    on('log', (e) => (this.log = [e, ...this.log].slice(0, 1000)))
    on('clients', (s) => (this.mcp = s))
    on('locks', (l) => (this.locks = l ?? {}))
    on('toast', (m) => this.toast(m))
    this.ready = true
    if (this.activeConn) this.loadSchema(this.activeConn)
  }

  toast(msg: string, ms = 2800) {
    clearTimeout(this.toastTimer)
    this.toastMsg = msg
    this.toastTimer = window.setTimeout(() => (this.toastMsg = null), ms)
  }

  async loadSchema(id: string, refresh = false) {
    if (!id || (this.schemaLoading[id] && !refresh)) return
    this.schemaLoading = { ...this.schemaLoading, [id]: true }
    try {
      const s = await api.schema(id, refresh)
      this.schemas = { ...this.schemas, [id]: s }
      if (refresh && this.columns[id]) {
        // A write can change columns. Load them again for completion.
        const { [id]: _, ...rest } = this.columns
        this.columns = rest
        this.loadColumns(id)
      }
      if (this.schemaErr[id]) {
        const { [id]: _, ...rest } = this.schemaErr
        this.schemaErr = rest
      }
    } catch (e) {
      this.schemaErr = { ...this.schemaErr, [id]: String(e) }
    } finally {
      this.schemaLoading = { ...this.schemaLoading, [id]: false }
    }
  }

  /** Loads what SQL completion needs. The editor calls it on focus. */
  prepareCompletion(id: string) {
    if (!id) return
    if (!this.schemas[id]) this.loadSchema(id)
    this.loadColumns(id)
  }

  async loadColumns(id: string) {
    if (this.columns[id] || this.columnsLoading.has(id)) return
    this.columnsLoading.add(id)
    try {
      const set = await api.columns(id)
      const map = new Map<string, Col[]>()
      for (const t of set.tables ?? []) map.set(t.schema + '.' + t.name, t.columns ?? [])
      this.columns = { ...this.columns, [id]: { map, truncated: set.truncated } }
    } catch {
      // Completion still offers tables and keywords without columns.
    } finally {
      this.columnsLoading.delete(id)
    }
  }

  private async loadTableColumns(id: string, t: TableSummary) {
    const key = id + '\u0000' + t.schema + '.' + t.name
    if (this.columnsLoading.has(key)) return
    this.columnsLoading.add(key)
    try {
      const info = await api.table(id, t.schema, t.name)
      const idx = this.columns[id]
      if (!idx) return
      const map = new Map(idx.map).set(t.schema + '.' + t.name, info.columns.map((c) => ({ name: c.name, type: c.type })))
      this.columns = { ...this.columns, [id]: { map, truncated: true } }
    } catch {
      // Leave the table without columns.
    } finally {
      this.columnsLoading.delete(key)
    }
  }

  /** The schema data for SQL completion on a connection. It changes when columns load. */
  completion(id: string, engine: string): Source {
    const idx = this.columns[id]
    return {
      engine,
      tables: this.tables(id),
      cols: (t, fetch = true) => {
        if (!idx) return undefined
        const hit = idx.map.get(t.schema + '.' + t.name)
        if (!hit && idx.truncated && fetch) this.loadTableColumns(id, t)
        return hit
      },
    }
  }

  tables(id: string): TableSummary[] {
    const s = this.schemas[id]
    return s?.groups?.flatMap((g) => g.tables) ?? []
  }

  setActive(id: string) {
    if (!id) return
    this.activeConn = id
    api.setLastConn(id)
    if (!this.schemas[id]) this.loadSchema(id)
  }

  session(id: string): Session {
    let s = this.sessions[id]
    if (!s) {
      s = new Session(id, this.drafts[id] ?? '')
      this.sessions[id] = s
    }
    return s
  }

  addHistory(h: HistoryItem) {
    this.history = [h, ...this.history].slice(0, 500)
  }

  /** Runs then() when writes are unlocked on conn, after the unlock prompt if needed. */
  requireWrite(conn: string, then?: () => void) {
    if (this.unlocked(conn)) {
      then?.()
      return
    }
    this.palette = false
    this.unlockReq = { conn, then }
  }

  async relock(conn: string) {
    await api.relock(conn)
    this.toast(`${this.conn(conn)?.name ?? conn} is read-only again`)
  }

  async saveApp(patch: Partial<AppSettings>) {
    this.config = { ...this.config, app: { ...this.config.app, ...patch } }
    this.config = await api.saveSettings(this.config.app, this.config.safety)
  }

  async saveSafety(patch: Partial<Safety>) {
    this.config = { ...this.config, safety: { ...this.config.safety, ...patch } }
    this.config = await api.saveSettings(this.config.app, this.config.safety)
  }

  toggleTheme() {
    this.saveApp({ theme: this.theme === 'dark' ? 'light' : 'dark' })
  }

  toggleLayout() {
    const next = this.layout === 'command' ? 'workspace' : 'command'
    this.palette = false
    this.saveApp({ layout: next })
    this.toast(next === 'workspace' ? 'Workspace layout' : 'Command layout')
  }

  /** Opens a table. In the command layout it shows the table view. In the
   * workspace layout it runs the query in the connection's editor. */
  openTable(conn: string, schema: string, table: string, opts: { rest?: string; sql?: string } = {}) {
    const c = this.conn(conn)
    if (!c) return
    const sql = opts.sql || `SELECT * FROM ${tableRef(c.engine, schema, table)}`
    this.palette = false
    this.setActive(conn)
    api.addRecent({ kind: 'table', conn, schema, table, title: table })
    this.recents = [{ kind: 'table' as const, conn, schema, table, title: table, at: Date.now() }, ...this.recents.filter((r) => !(r.kind === 'table' && r.conn === conn && r.table === table))].slice(0, 24)
    if (this.layout === 'command') {
      this.view = { kind: 'table', conn, schema, table, rest: opts.rest ?? '', sql }
      return
    }
    this.rail = 'tables'
    const s = this.session(conn)
    if (!s.dirty || !s.sql.trim()) {
      s.setSQL(sql + ';', false)
      s.run({ sql })
    } else {
      s.run({ sql, label: table })
    }
  }

  /** Opens SQL in the editor of a connection. */
  openQuery(conn: string, sql?: string, run = false) {
    if (!this.conn(conn)) return
    this.palette = false
    this.setActive(conn)
    const s = this.session(conn)
    if (sql !== undefined) s.setSQL(sql)
    if (this.layout === 'command') this.view = { kind: 'query', conn }
    else this.rail = 'tables'
    if (sql?.trim()) {
      const title = sql.trim().split('\n')[0].replace(/^--\s*/, '').slice(0, 60)
      api.addRecent({ kind: 'query', conn, title, sql })
      this.recents = [{ kind: 'query' as const, conn, title, sql, at: Date.now() }, ...this.recents.filter((r) => !(r.kind === 'query' && r.sql === sql))].slice(0, 24)
    }
    if (run) s.run()
  }

  openView(kind: 'log' | 'mcp' | 'settings', q = '') {
    this.palette = false
    if (this.layout === 'command') {
      this.view = kind === 'log' ? { kind, q } : { kind }
      api.addRecent({ kind: 'view', conn: this.activeConn, title: { log: 'Activity log', mcp: 'MCP server', settings: 'Settings' }[kind], view: kind })
    } else {
      this.rail = kind
      if (kind === 'log') this.logQuery = q
    }
  }

  logQuery = $state('')

  home() {
    this.view = { kind: 'home' }
  }

  async decide(ids: string[], approve: boolean) {
    const n = await api.decide(ids, approve)
    if (n) this.toast(`${approve ? 'Approved' : 'Denied'} ${plural(n, 'write')}`)
  }

  statusLine() {
    const n = this.mcp.clients?.filter((c) => c.connected && !c.blocked).length ?? 0
    return this.mcp.running ? `MCP · ${plural(n, 'client')} · ${fmtN(this.queue.length)} waiting` : 'MCP server off'
  }
}

export const app = new Store()
