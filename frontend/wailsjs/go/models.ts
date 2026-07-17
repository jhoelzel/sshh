export namespace bridge {

	export class FrontendLeaseDTO {
	    id: string;
	    expiresAt: string;

	    static createFrom(source: any = {}) {
	        return new FrontendLeaseDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
	export class ProfileDTO {
	    id: string;
	    name: string;
	    protocol: string;
	    host: string;
	    port: number;
	    username: string;
	    authentication: string;
	    identityFile: string;
	    shell: string;
	    arguments: string[];
	    workingDirectory: string;
	    environment: Record<string, string>;
	    tags: string[];
	    group: string;
	    favorite: boolean;
	    endpoint: string;
	    connectable: boolean;

	    static createFrom(source: any = {}) {
	        return new ProfileDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.protocol = source["protocol"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authentication = source["authentication"];
	        this.identityFile = source["identityFile"];
	        this.shell = source["shell"];
	        this.arguments = source["arguments"];
	        this.workingDirectory = source["workingDirectory"];
	        this.environment = source["environment"];
	        this.tags = source["tags"];
	        this.group = source["group"];
	        this.favorite = source["favorite"];
	        this.endpoint = source["endpoint"];
	        this.connectable = source["connectable"];
	    }
	}
	export class ProfileExportDTO {
	    cancelled: boolean;
	    filename: string;
	    exported: number;

	    static createFrom(source: any = {}) {
	        return new ProfileExportDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cancelled = source["cancelled"];
	        this.filename = source["filename"];
	        this.exported = source["exported"];
	    }
	}
	export class ProfileImportDTO {
	    cancelled: boolean;
	    format: string;
	    filename: string;
	    imported: ProfileDTO[];
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new ProfileImportDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cancelled = source["cancelled"];
	        this.format = source["format"];
	        this.filename = source["filename"];
	        this.imported = this.convertValues(source["imported"], ProfileDTO);
	        this.warnings = source["warnings"];
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
	export class ProfileInputDTO {
	    id: string;
	    name: string;
	    protocol: string;
	    host: string;
	    port: number;
	    username: string;
	    authentication: string;
	    identityFile: string;
	    shell: string;
	    arguments: string[];
	    workingDirectory: string;
	    environment: Record<string, string>;
	    tags: string[];
	    group: string;
	    favorite: boolean;

	    static createFrom(source: any = {}) {
	        return new ProfileInputDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.protocol = source["protocol"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authentication = source["authentication"];
	        this.identityFile = source["identityFile"];
	        this.shell = source["shell"];
	        this.arguments = source["arguments"];
	        this.workingDirectory = source["workingDirectory"];
	        this.environment = source["environment"];
	        this.tags = source["tags"];
	        this.group = source["group"];
	        this.favorite = source["favorite"];
	    }
	}
	export class QuickSSHInputDTO {
	    host: string;
	    port: number;
	    username: string;
	    authentication: string;
	    identityFile: string;

	    static createFrom(source: any = {}) {
	        return new QuickSSHInputDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authentication = source["authentication"];
	        this.identityFile = source["identityFile"];
	    }
	}
	export class SSHHostKeyDTO {
	    status: string;
	    host: string;
	    address: string;
	    algorithm: string;
	    fingerprint: string;
	    challengeId: string;

	    static createFrom(source: any = {}) {
	        return new SSHHostKeyDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.host = source["host"];
	        this.address = source["address"];
	        this.algorithm = source["algorithm"];
	        this.fingerprint = source["fingerprint"];
	        this.challengeId = source["challengeId"];
	    }
	}
	export class QuickSSHProbeDTO {
	    profile: ProfileDTO;
	    hostKey: SSHHostKeyDTO;

	    static createFrom(source: any = {}) {
	        return new QuickSSHProbeDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], ProfileDTO);
	        this.hostKey = this.convertValues(source["hostKey"], SSHHostKeyDTO);
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
	export class RemotePathFavoriteDTO {
	    id: string;
	    profileId: string;
	    path: string;
	    createdAt: string;

	    static createFrom(source: any = {}) {
	        return new RemotePathFavoriteDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.profileId = source["profileId"];
	        this.path = source["path"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class SSHAuthenticationDTO {
	    secret: string;
	    identityFile: string;

	    static createFrom(source: any = {}) {
	        return new SSHAuthenticationDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.secret = source["secret"];
	        this.identityFile = source["identityFile"];
	    }
	}
	export class SSHCredentialsDTO {
	    password: string;
	    passphrase: string;

	    static createFrom(source: any = {}) {
	        return new SSHCredentialsDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.password = source["password"];
	        this.passphrase = source["passphrase"];
	    }
	}

	export class TerminalSettingsDTO {
	    fontFamily: string;
	    fontSize: number;
	    lineHeight: number;
	    cursorStyle: string;
	    cursorBlink: boolean;
	    scrollback: number;
	    bell: boolean;

	    static createFrom(source: any = {}) {
	        return new TerminalSettingsDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fontFamily = source["fontFamily"];
	        this.fontSize = source["fontSize"];
	        this.lineHeight = source["lineHeight"];
	        this.cursorStyle = source["cursorStyle"];
	        this.cursorBlink = source["cursorBlink"];
	        this.scrollback = source["scrollback"];
	        this.bell = source["bell"];
	    }
	}
	export class SettingsDTO {
	    terminal: TerminalSettingsDTO;

	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.terminal = this.convertValues(source["terminal"], TerminalSettingsDTO);
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
	export class SnippetDTO {
	    id: string;
	    name: string;
	    folder: string;
	    tags: string[];
	    body: string;
	    variables: string[];
	    createdAt: string;
	    updatedAt: string;

	    static createFrom(source: any = {}) {
	        return new SnippetDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.folder = source["folder"];
	        this.tags = source["tags"];
	        this.body = source["body"];
	        this.variables = source["variables"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class SnippetInputDTO {
	    id: string;
	    name: string;
	    folder: string;
	    tags: string[];
	    body: string;

	    static createFrom(source: any = {}) {
	        return new SnippetInputDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.folder = source["folder"];
	        this.tags = source["tags"];
	        this.body = source["body"];
	    }
	}
	export class SnippetPreviewDTO {
	    text: string;
	    variables: string[];

	    static createFrom(source: any = {}) {
	        return new SnippetPreviewDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.variables = source["variables"];
	    }
	}

	export class TerminalTextExportDTO {
	    cancelled: boolean;
	    filename: string;
	    bytes: number;

	    static createFrom(source: any = {}) {
	        return new TerminalTextExportDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cancelled = source["cancelled"];
	        this.filename = source["filename"];
	        this.bytes = source["bytes"];
	    }
	}
	export class TunnelInputDTO {
	    id: string;
	    name: string;
	    profileId: string;
	    kind: string;
	    bindAddress: string;
	    bindPort: number;
	    destinationHost: string;
	    destinationPort: number;
	    autoStart: boolean;
	    reconnect: boolean;

	    static createFrom(source: any = {}) {
	        return new TunnelInputDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.profileId = source["profileId"];
	        this.kind = source["kind"];
	        this.bindAddress = source["bindAddress"];
	        this.bindPort = source["bindPort"];
	        this.destinationHost = source["destinationHost"];
	        this.destinationPort = source["destinationPort"];
	        this.autoStart = source["autoStart"];
	        this.reconnect = source["reconnect"];
	    }
	}
	export class WorkspaceTabDTO {
	    profileId: string;
	    title: string;
	    endpoint: string;

	    static createFrom(source: any = {}) {
	        return new WorkspaceTabDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.title = source["title"];
	        this.endpoint = source["endpoint"];
	    }
	}
	export class WorkspaceLayoutDTO {
	    id: string;
	    name: string;
	    tabs: WorkspaceTabDTO[];
	    activeTab: number;
	    createdAt: string;
	    updatedAt: string;

	    static createFrom(source: any = {}) {
	        return new WorkspaceLayoutDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.tabs = this.convertValues(source["tabs"], WorkspaceTabDTO);
	        this.activeTab = source["activeTab"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	export class WorkspaceLayoutInputDTO {
	    id: string;
	    name: string;
	    tabs: WorkspaceTabDTO[];
	    activeTab: number;

	    static createFrom(source: any = {}) {
	        return new WorkspaceLayoutInputDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.tabs = this.convertValues(source["tabs"], WorkspaceTabDTO);
	        this.activeTab = source["activeTab"];
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

export namespace filetransfer {

	export class Entry {
	    name: string;
	    path: string;
	    directory: boolean;
	    symlink: boolean;
	    size: number;
	    mode: number;
	    modifiedAt: string;

	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.directory = source["directory"];
	        this.symlink = source["symlink"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.modifiedAt = source["modifiedAt"];
	    }
	}
	export class Session {
	    id: string;
	    leaseId: string;
	    profileId: string;
	    root: string;
	    openedAt: string;

	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.leaseId = source["leaseId"];
	        this.profileId = source["profileId"];
	        this.root = source["root"];
	        this.openedAt = source["openedAt"];
	    }
	}
	export class Transfer {
	    id: string;
	    leaseId: string;
	    sessionId: string;
	    direction: string;
	    source: string;
	    destination: string;
	    bytes: number;
	    total: number;
	    state: string;
	    message: string;
	    startedAt: string;
	    finishedAt: string;

	    static createFrom(source: any = {}) {
	        return new Transfer(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.leaseId = source["leaseId"];
	        this.sessionId = source["sessionId"];
	        this.direction = source["direction"];
	        this.source = source["source"];
	        this.destination = source["destination"];
	        this.bytes = source["bytes"];
	        this.total = source["total"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	}

}

export namespace session {

	export class Session {
	    id: string;
	    generation: number;
	    leaseId: string;
	    profileId: string;
	    title: string;
	    state: string;
	    columns: number;
	    rows: number;
	    startedAt: string;

	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.generation = source["generation"];
	        this.leaseId = source["leaseId"];
	        this.profileId = source["profileId"];
	        this.title = source["title"];
	        this.state = source["state"];
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class SessionLogStatus {
	    leaseId: string;
	    sessionId: string;
	    generation: number;
	    active: boolean;
	    path: string;
	    bytesWritten: number;
	    timestampLines: boolean;
	    startedAt: string;
	    stoppedAt: string;
	    message: string;

	    static createFrom(source: any = {}) {
	        return new SessionLogStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.leaseId = source["leaseId"];
	        this.sessionId = source["sessionId"];
	        this.generation = source["generation"];
	        this.active = source["active"];
	        this.path = source["path"];
	        this.bytesWritten = source["bytesWritten"];
	        this.timestampLines = source["timestampLines"];
	        this.startedAt = source["startedAt"];
	        this.stoppedAt = source["stoppedAt"];
	        this.message = source["message"];
	    }
	}

}

export namespace tunnel {

	export class Config {
	    id: string;
	    name: string;
	    profileId: string;
	    kind: string;
	    bindAddress: string;
	    bindPort: number;
	    destinationHost: string;
	    destinationPort: number;
	    autoStart: boolean;
	    reconnect: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;

	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.profileId = source["profileId"];
	        this.kind = source["kind"];
	        this.bindAddress = source["bindAddress"];
	        this.bindPort = source["bindPort"];
	        this.destinationHost = source["destinationHost"];
	        this.destinationPort = source["destinationPort"];
	        this.autoStart = source["autoStart"];
	        this.reconnect = source["reconnect"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class Snapshot {
	    configId: string;
	    leaseId: string;
	    state: string;
	    boundAddress: string;
	    message: string;
	    startedAt: string;
	    updatedAt: string;

	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configId = source["configId"];
	        this.leaseId = source["leaseId"];
	        this.state = source["state"];
	        this.boundAddress = source["boundAddress"];
	        this.message = source["message"];
	        this.startedAt = source["startedAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}
