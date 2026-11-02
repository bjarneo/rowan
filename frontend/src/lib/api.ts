// Typed wrappers around the generated Wails bindings.
import * as Go from './wailsjs/go/main/App'
import { EventsOn, ClipboardSetText } from './wailsjs/runtime/runtime'
import type {
  Bootstrap, Config, Connection, SchemaInfo, ColumnSet, TableInfo, RunResult, PreviewResult, Inspection,
  QueueItem, LogEntry, McpStatus, Policy, AppSettings, Safety, McpSettings, Recent, HistoryItem,
} from './types'

const call = <T>(p: Promise<unknown>) => p as Promise<T>

export const api = {
  bootstrap: () => call<Bootstrap>(Go.Bootstrap()),
  saveSettings: (app: AppSettings, safety: Safety) => call<Config>(Go.SaveSettings(app as any, safety as any)),
  saveConnection: (form: Connection & { password: string; passwordChanged: boolean }) => call<Connection>(Go.SaveConnection(form as any)),
  deleteConnection: (id: string) => call<void>(Go.DeleteConnection(id)),
  testConnection: (form: Connection & { password: string; passwordChanged: boolean }) =>
    call<{ ok: boolean; message: string }>(Go.TestConnection(form as any)),
  chooseFile: (title: string) => call<string>(Go.ChooseFile(title)),
  schema: (conn: string, refresh = false) => call<SchemaInfo>(Go.Schema(conn, refresh)),
  table: (conn: string, schema: string, name: string) => call<TableInfo>(Go.Table(conn, schema, name)),
  columns: (conn: string) => call<ColumnSet>(Go.Columns(conn)),
  run: (req: { runId: string; conn: string; sql: string; limit: number; force: boolean }) => call<RunResult>(Go.Run(req as any)),
  explain: (req: { runId: string; conn: string; sql: string; limit: number; force: boolean }) => call<RunResult>(Go.Explain(req as any)),
  cancel: (runId: string) => call<void>(Go.Cancel(runId)),
  preview: (req: { conn: string; schema: string; table: string; rest: string; limit: number }) => call<PreviewResult>(Go.Preview(req as any)),
  previewSQL: (conn: string, sql: string) => call<RunResult>(Go.PreviewSQL(conn, sql)),
  inspect: (req: { conn: string; schema: string; table: string; row: Record<string, string | null> }) => call<Inspection>(Go.Inspect(req as any)),
  unlock: (conn: string, minutes: number, typed: string) => call<void>(Go.Unlock(conn, minutes, typed)),
  relock: (conn: string) => call<void>(Go.Relock(conn)),
  decide: (ids: string[], approve: boolean) => call<number>(Go.Decide(ids, approve)),
  setPolicy: (conn: string, p: Policy) => call<Config>(Go.SetPolicy(conn, p as any)),
  setMcp: (m: McpSettings) => call<McpStatus>(Go.SetMCP(m as any)),
  mcpSetup: (client: string) => call<{ file: string; code: string }>(Go.McpSetupFor(client)),
  activityLog: (n: number) => call<LogEntry[]>(Go.ActivityLog(n)),
  exportResult: (id: string, format: string) => call<string>(Go.Export(id, format)),
  saveSnippet: (name: string, sql: string) => call<Config>(Go.SaveSnippet(name, sql)),
  deleteSnippet: (id: string) => call<Config>(Go.DeleteSnippet(id)),
  addRecent: (r: Omit<Recent, 'at'>) => call<void>(Go.AddRecent({ ...r, at: 0 } as any)),
  setDraft: (conn: string, sql: string) => call<void>(Go.SetDraft(conn, sql)),
  setLastConn: (id: string) => call<void>(Go.SetLastConn(id)),
  history: () => call<HistoryItem[]>(Go.History()),
  copy: (text: string) => ClipboardSetText(text),
}

export function on(event: 'queue', fn: (q: QueueItem[] | null) => void): () => void
export function on(event: 'log', fn: (e: LogEntry) => void): () => void
export function on(event: 'clients', fn: (s: McpStatus) => void): () => void
export function on(event: 'locks', fn: (l: Record<string, number>) => void): () => void
export function on(event: 'toast', fn: (msg: string) => void): () => void
export function on(event: string, fn: (data: any) => void): () => void {
  return EventsOn(event, fn)
}
