export type SessionState = 'starting' | 'running' | 'closing' | 'exited' | 'failed' | 'closed'
export type ProfileProtocol = 'local' | 'ssh'
export type SSHAuthentication = 'auto' | 'agent' | 'key' | 'password'
export type SSHSecretRequirement = 'none' | 'password' | 'passphrase'

export interface Profile {
  id: string
  name: string
  protocol: ProfileProtocol
  host: string
  port: number
  username: string
  authentication: SSHAuthentication
  identityFile: string
  shell: string
  arguments: string[]
  workingDirectory: string
  environment: Record<string, string>
  tags: string[]
  group: string
  favorite: boolean
  endpoint: string
  connectable: boolean
}

export type ProfileInput = Omit<Profile, 'endpoint' | 'connectable'>

export interface ProfileImportResult {
  cancelled: boolean
  format: string
  filename: string
  imported: Profile[]
  warnings: string[]
}

export interface ProfileExportResult {
  cancelled: boolean
  filename: string
  exported: number
}

export type ProfileExchangeResult =
  | { kind: 'import'; result: ProfileImportResult }
  | { kind: 'export'; result: ProfileExportResult }

export interface SSHHostKey {
  status: 'known' | 'unknown' | 'changed'
  host: string
  address: string
  algorithm: string
  fingerprint: string
  challengeId: string
}

export interface SSHAuthenticationInfo {
  secret: SSHSecretRequirement
  identityFile: string
}

export interface SSHCredentials {
  password: string
  passphrase: string
}

export interface QuickSSHInput {
  host: string
  port: number
  username: string
  authentication: SSHAuthentication
  identityFile: string
}

export interface QuickSSHProbe {
  profile: Profile
  hostKey: SSHHostKey
}

export interface FileSession {
  id: string
  leaseId: string
  profileId: string
  root: string
  openedAt: string
}

export interface RemoteFile {
  name: string
  path: string
  directory: boolean
  symlink: boolean
  size: number
  mode: number
  modifiedAt: string
}

export interface RemotePathFavorite {
  id: string
  profileId: string
  path: string
  createdAt: string
}

export type TransferState = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'skipped'

export interface Transfer {
  id: string
  leaseId: string
  sessionId: string
  direction: 'download' | 'upload'
  source: string
  destination: string
  bytes: number
  total: number
  state: TransferState
  message: string
  startedAt: string
  finishedAt: string
}

export type TunnelKind = 'local' | 'remote' | 'dynamic'
export type TunnelState = 'starting' | 'active' | 'retrying' | 'failed' | 'stopped'

export interface TunnelConfig {
  id: string
  name: string
  profileId: string
  kind: TunnelKind
  bindAddress: string
  bindPort: number
  destinationHost: string
  destinationPort: number
  autoStart: boolean
  reconnect: boolean
  createdAt: string
  updatedAt: string
}

export type TunnelInput = Omit<TunnelConfig, 'createdAt' | 'updatedAt'>

export interface TunnelSnapshot {
  configId: string
  leaseId: string
  state: TunnelState
  boundAddress: string
  message: string
  startedAt: string
  updatedAt: string
}

export interface FrontendLease {
  id: string
  expiresAt: string
}

export interface Session {
  id: string
  generation: number
  leaseId: string
  profileId: string
  title: string
  state: SessionState
  columns: number
  rows: number
  startedAt: string
}

export interface TerminalOutput {
  leaseId: string
  sessionId: string
  generation: number
  sequence: number
  endOffset: number
  byteCount: number
  payload: string
  final: boolean
}

export interface TerminalTextExportResult {
  cancelled: boolean
  filename: string
  bytes: number
}

export interface SessionStateEvent {
  leaseId: string
  sessionId: string
  generation: number
  title: string
  state: SessionState
  exitCode?: number
  signal?: string
  message?: string
}

export interface Snippet {
  id: string
  name: string
  folder: string
  tags: string[]
  body: string
  variables: string[]
  createdAt: string
  updatedAt: string
}

export type SnippetInput = Omit<Snippet, 'variables' | 'createdAt' | 'updatedAt'>

export interface SnippetPreview {
  text: string
  variables: string[]
}

export interface WorkspaceTab {
  profileId: string
  title: string
  endpoint: string
}

export interface WorkspaceLayout {
  id: string
  name: string
  tabs: WorkspaceTab[]
  activeTab: number
  createdAt: string
  updatedAt: string
}

export type WorkspaceLayoutInput = Omit<WorkspaceLayout, 'createdAt' | 'updatedAt'>

export interface SessionLogStatus {
  leaseId: string
  sessionId: string
  generation: number
  active: boolean
  path: string
  bytesWritten: number
  timestampLines: boolean
  startedAt: string
  stoppedAt: string
  message: string
}

export type TerminalFontFamily = 'system-mono' | 'menlo' | 'monaco'
export type TerminalCursorStyle = 'block' | 'bar' | 'underline'

export interface TerminalSettings {
  fontFamily: TerminalFontFamily
  fontSize: number
  lineHeight: number
  cursorStyle: TerminalCursorStyle
  cursorBlink: boolean
  scrollback: number
  bell: boolean
}

export interface NotificationSettings {
  enabled: boolean
  transferCompleted: boolean
  unexpectedDisconnect: boolean
  longTransferSeconds: number
}

export interface ConnectionSettings {
  connectTimeoutSeconds: number
  keepAliveEnabled: boolean
  keepAliveIntervalSeconds: number
  keepAliveMaxFailures: number
}

export interface NotificationStatus {
  available: boolean
  authorized: boolean
  message: string
}

export type TransferCollisionPolicy = 'ask' | 'overwrite' | 'skip' | 'rename'

export interface TransferSettings {
  concurrency: number
  collisionPolicy: TransferCollisionPolicy
  keepPartialFiles: boolean
}

export interface AppSettings {
  terminal: TerminalSettings
  connection: ConnectionSettings
  notifications: NotificationSettings
  transfers: TransferSettings
}
