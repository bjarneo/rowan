export namespace agent {
	
	export class Change {
	    column: string;
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new Change(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.column = source["column"];
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class ClientInfo {
	    name: string;
	    transport: string;
	    since: number;
	    lastSeen: number;
	    calls: number;
	    connected: boolean;
	    blocked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClientInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.transport = source["transport"];
	        this.since = source["since"];
	        this.lastSeen = source["lastSeen"];
	        this.calls = source["calls"];
	        this.connected = source["connected"];
	        this.blocked = source["blocked"];
	    }
	}
	export class Entry {
	    id: string;
	    at: number;
	    client: string;
	    tool: string;
	    conn: string;
	    sql: string;
	    rows: number;
	    ms: number;
	    status: string;
	    note: string;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.client = source["client"];
	        this.tool = source["tool"];
	        this.conn = source["conn"];
	        this.sql = source["sql"];
	        this.rows = source["rows"];
	        this.ms = source["ms"];
	        this.status = source["status"];
	        this.note = source["note"];
	        this.reason = source["reason"];
	    }
	}
	export class Item {
	    id: string;
	    at: number;
	    client: string;
	    conn: string;
	    reason: string;
	    sql: string;
	    impact: string;
	    changes: Change[];
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.client = source["client"];
	        this.conn = source["conn"];
	        this.reason = source["reason"];
	        this.sql = source["sql"];
	        this.impact = source["impact"];
	        this.changes = this.convertValues(source["changes"], Change);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Status {
	    running: boolean;
	    port: number;
	    error: string;
	    clients: ClientInfo[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.error = source["error"];
	        this.clients = this.convertValues(source["clients"], ClientInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace config {
	
	export class App {
	    theme: string;
	    layout: string;
	    accent: string;
	    density: string;
	    rowLimit: number;
	
	    static createFrom(source: any = {}) {
	        return new App(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.layout = source["layout"];
	        this.accent = source["accent"];
	        this.density = source["density"];
	        this.rowLimit = source["rowLimit"];
	    }
	}
	export class Snippet {
	    id: string;
	    name: string;
	    sql: string;
	
	    static createFrom(source: any = {}) {
	        return new Snippet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sql = source["sql"];
	    }
	}
	export class Policy {
	    exposed: boolean;
	    access: string;
	    approveWrites: boolean;
	    rowLimit: number;
	    tables: string[];
	
	    static createFrom(source: any = {}) {
	        return new Policy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exposed = source["exposed"];
	        this.access = source["access"];
	        this.approveWrites = source["approveWrites"];
	        this.rowLimit = source["rowLimit"];
	        this.tables = source["tables"];
	    }
	}
	export class SSH {
	    enabled: boolean;
	    host: string;
	    port: number;
	    user: string;
	    key: string;
	
	    static createFrom(source: any = {}) {
	        return new SSH(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.key = source["key"];
	    }
	}
	export class Connection {
	    id: string;
	    name: string;
	    engine: string;
	    host: string;
	    port: number;
	    database: string;
	    user: string;
	    hasPassword: boolean;
	    file: string;
	    sslMode: string;
	    sslRootCert: string;
	    color: string;
	    production: boolean;
	    ssh: SSH;
	    mcp: Policy;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.engine = source["engine"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.database = source["database"];
	        this.user = source["user"];
	        this.hasPassword = source["hasPassword"];
	        this.file = source["file"];
	        this.sslMode = source["sslMode"];
	        this.sslRootCert = source["sslRootCert"];
	        this.color = source["color"];
	        this.production = source["production"];
	        this.ssh = this.convertValues(source["ssh"], SSH);
	        this.mcp = this.convertValues(source["mcp"], Policy);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MCP {
	    enabled: boolean;
	    httpPort: number;
	    tools: string[];
	    blockedClients: string[];
	
	    static createFrom(source: any = {}) {
	        return new MCP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.httpPort = source["httpPort"];
	        this.tools = source["tools"];
	        this.blockedClients = source["blockedClients"];
	    }
	}
	export class Safety {
	    relockAfter: string;
	    confirmProductionUnlock: boolean;
	    confirmDestructive: boolean;
	    statementTimeout: string;
	
	    static createFrom(source: any = {}) {
	        return new Safety(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relockAfter = source["relockAfter"];
	        this.confirmProductionUnlock = source["confirmProductionUnlock"];
	        this.confirmDestructive = source["confirmDestructive"];
	        this.statementTimeout = source["statementTimeout"];
	    }
	}
	export class Config {
	    app: App;
	    safety: Safety;
	    mcp: MCP;
	    connections: Connection[];
	    snippets: Snippet[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app = this.convertValues(source["app"], App);
	        this.safety = this.convertValues(source["safety"], Safety);
	        this.mcp = this.convertValues(source["mcp"], MCP);
	        this.connections = this.convertValues(source["connections"], Connection);
	        this.snippets = this.convertValues(source["snippets"], Snippet);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	

}

export namespace db {
	
	export class ColBrief {
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new ColBrief(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class Column {
	    name: string;
	    type: string;
	    numeric: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Column(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.numeric = source["numeric"];
	    }
	}
	export class Ref {
	    schema: string;
	    table: string;
	    column: string;
	    local: string;
	
	    static createFrom(source: any = {}) {
	        return new Ref(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.column = source["column"];
	        this.local = source["local"];
	    }
	}
	export class ColumnInfo {
	    name: string;
	    type: string;
	    nullable: boolean;
	    default: string;
	    pk: boolean;
	    unique: boolean;
	    fk?: Ref;
	    numeric: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	        this.default = source["default"];
	        this.pk = source["pk"];
	        this.unique = source["unique"];
	        this.fk = this.convertValues(source["fk"], Ref);
	        this.numeric = source["numeric"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableColumns {
	    schema: string;
	    name: string;
	    columns: ColBrief[];
	
	    static createFrom(source: any = {}) {
	        return new TableColumns(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.columns = this.convertValues(source["columns"], ColBrief);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ColumnSet {
	    tables: TableColumns[];
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tables = this.convertValues(source["tables"], TableColumns);
	        this.truncated = source["truncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableSummary {
	    schema: string;
	    name: string;
	    kind: string;
	    rows: number;
	    estimate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TableSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.rows = source["rows"];
	        this.estimate = source["estimate"];
	    }
	}
	export class Group {
	    name: string;
	    tables: TableSummary[];
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.tables = this.convertValues(source["tables"], TableSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Index {
	    name: string;
	    columns: string;
	    unique: boolean;
	    primary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Index(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.unique = source["unique"];
	        this.primary = source["primary"];
	    }
	}
	
	export class Schema {
	    version: string;
	    groups: Group[];
	
	    static createFrom(source: any = {}) {
	        return new Schema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.groups = this.convertValues(source["groups"], Group);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Table {
	    schema: string;
	    name: string;
	    kind: string;
	    columns: ColumnInfo[];
	    indexes: Index[];
	    referencedBy: Ref[];
	
	    static createFrom(source: any = {}) {
	        return new Table(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.columns = this.convertValues(source["columns"], ColumnInfo);
	        this.indexes = this.convertValues(source["indexes"], Index);
	        this.referencedBy = this.convertValues(source["referencedBy"], Ref);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace main {
	
	export class Bootstrap {
	    config: config.Config;
	    state: state.State;
	    locks: Record<string, number>;
	    queue: agent.Item[];
	    log: agent.Entry[];
	    mcp: agent.Status;
	    configPath: string;
	    platform: string;
	    firstRun: boolean;
	    notice: string;
	
	    static createFrom(source: any = {}) {
	        return new Bootstrap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], config.Config);
	        this.state = this.convertValues(source["state"], state.State);
	        this.locks = source["locks"];
	        this.queue = this.convertValues(source["queue"], agent.Item);
	        this.log = this.convertValues(source["log"], agent.Entry);
	        this.mcp = this.convertValues(source["mcp"], agent.Status);
	        this.configPath = source["configPath"];
	        this.platform = source["platform"];
	        this.firstRun = source["firstRun"];
	        this.notice = source["notice"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConnForm {
	    id: string;
	    name: string;
	    engine: string;
	    host: string;
	    port: number;
	    database: string;
	    user: string;
	    hasPassword: boolean;
	    file: string;
	    sslMode: string;
	    sslRootCert: string;
	    color: string;
	    production: boolean;
	    ssh: config.SSH;
	    mcp: config.Policy;
	    password: string;
	    passwordChanged: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConnForm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.engine = source["engine"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.database = source["database"];
	        this.user = source["user"];
	        this.hasPassword = source["hasPassword"];
	        this.file = source["file"];
	        this.sslMode = source["sslMode"];
	        this.sslRootCert = source["sslRootCert"];
	        this.color = source["color"];
	        this.production = source["production"];
	        this.ssh = this.convertValues(source["ssh"], config.SSH);
	        this.mcp = this.convertValues(source["mcp"], config.Policy);
	        this.password = source["password"];
	        this.passwordChanged = source["passwordChanged"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InspectRequest {
	    conn: string;
	    schema: string;
	    table: string;
	    row: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new InspectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conn = source["conn"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.row = source["row"];
	    }
	}
	export class Link {
	    column: string;
	    table: string;
	    schema: string;
	    label: string;
	    count: number;
	    capped: boolean;
	    sql: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.column = source["column"];
	        this.table = source["table"];
	        this.schema = source["schema"];
	        this.label = source["label"];
	        this.count = source["count"];
	        this.capped = source["capped"];
	        this.sql = source["sql"];
	    }
	}
	export class Inspection {
	    fks: Record<string, Link>;
	    refBy: Link[];
	
	    static createFrom(source: any = {}) {
	        return new Inspection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fks = this.convertValues(source["fks"], Link, true);
	        this.refBy = this.convertValues(source["refBy"], Link);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class McpSetup {
	    file: string;
	    code: string;
	
	    static createFrom(source: any = {}) {
	        return new McpSetup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.code = source["code"];
	    }
	}
	export class PreviewRequest {
	    conn: string;
	    schema: string;
	    table: string;
	    rest: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new PreviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conn = source["conn"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.rest = source["rest"];
	        this.limit = source["limit"];
	    }
	}
	export class PreviewResult {
	    schema: string;
	    table: string;
	    columns: db.Column[];
	    rows: any[][];
	    interp: search.Interp;
	    matched: number;
	    matchedCapped: boolean;
	    total: number;
	    totalEstimate: boolean;
	    sql: string;
	    where: string;
	    error: string;
	    canceled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PreviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.columns = this.convertValues(source["columns"], db.Column);
	        this.rows = source["rows"];
	        this.interp = this.convertValues(source["interp"], search.Interp);
	        this.matched = source["matched"];
	        this.matchedCapped = source["matchedCapped"];
	        this.total = source["total"];
	        this.totalEstimate = source["totalEstimate"];
	        this.sql = source["sql"];
	        this.where = source["where"];
	        this.error = source["error"];
	        this.canceled = source["canceled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunRequest {
	    runId: string;
	    conn: string;
	    sql: string;
	    limit: number;
	    force: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.conn = source["conn"];
	        this.sql = source["sql"];
	        this.limit = source["limit"];
	        this.force = source["force"];
	    }
	}
	export class SourceRef {
	    schema: string;
	    table: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.table = source["table"];
	    }
	}
	export class RunResult {
	    id: string;
	    columns: db.Column[];
	    rows: any[][];
	    truncated: boolean;
	    affected: number;
	    message: string;
	    ms: number;
	    statements: number;
	    write: boolean;
	    error: string;
	    locked: boolean;
	    confirm: string;
	    source?: SourceRef;
	
	    static createFrom(source: any = {}) {
	        return new RunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.columns = this.convertValues(source["columns"], db.Column);
	        this.rows = source["rows"];
	        this.truncated = source["truncated"];
	        this.affected = source["affected"];
	        this.message = source["message"];
	        this.ms = source["ms"];
	        this.statements = source["statements"];
	        this.write = source["write"];
	        this.error = source["error"];
	        this.locked = source["locked"];
	        this.confirm = source["confirm"];
	        this.source = this.convertValues(source["source"], SourceRef);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class TestResult {
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}

}

export namespace search {
	
	export class Filter {
	    column: string;
	    op: string;
	    value: string;
	    source: string;
	    guess: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Filter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.column = source["column"];
	        this.op = source["op"];
	        this.value = source["value"];
	        this.source = source["source"];
	        this.guess = source["guess"];
	    }
	}
	export class Interp {
	    filters: Filter[];
	    unresolved: string[];
	    note: string;
	    canonical: string;
	
	    static createFrom(source: any = {}) {
	        return new Interp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filters = this.convertValues(source["filters"], Filter);
	        this.unresolved = source["unresolved"];
	        this.note = source["note"];
	        this.canonical = source["canonical"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace state {
	
	export class HistoryItem {
	    id: string;
	    sql: string;
	    conn: string;
	    at: number;
	    rows: number;
	    ms: number;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sql = source["sql"];
	        this.conn = source["conn"];
	        this.at = source["at"];
	        this.rows = source["rows"];
	        this.ms = source["ms"];
	        this.status = source["status"];
	    }
	}
	export class Recent {
	    kind: string;
	    conn: string;
	    schema?: string;
	    table?: string;
	    title: string;
	    sql?: string;
	    view?: string;
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new Recent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.conn = source["conn"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.title = source["title"];
	        this.sql = source["sql"];
	        this.view = source["view"];
	        this.at = source["at"];
	    }
	}
	export class State {
	    history: HistoryItem[];
	    recents: Recent[];
	    drafts: Record<string, string>;
	    lastConn: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.history = this.convertValues(source["history"], HistoryItem);
	        this.recents = this.convertValues(source["recents"], Recent);
	        this.drafts = source["drafts"];
	        this.lastConn = source["lastConn"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

