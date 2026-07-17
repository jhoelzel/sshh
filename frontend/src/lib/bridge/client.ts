import {
  AcknowledgeTerminalOutput,
  ActivateTerminal,
  AttachFrontend,
  CancelTransfer,
  ChmodRemotePath,
  CloseSFTP,
  CloseTerminal,
  ConfirmApplicationClose,
  CreateProfile,
  CreateRemoteDirectory,
  CreateSnippet,
  CreateTunnel,
  DeleteProfile,
  DeleteRemotePath,
  DeleteSnippet,
  DeleteTunnel,
  DuplicateProfile,
  ExportProfiles,
  GetSettings,
  ListProfiles,
  ListRemoteFiles,
  ListSnippets,
  ListTransfers,
  ListTunnelStates,
  ListTunnels,
  InspectSSHAuthentication,
  ImportProfiles,
  InspectQuickSSHAuthentication,
  OpenLocalTerminal,
  OpenQuickSSHTerminal,
  OpenSFTP,
  OpenSSHTerminal,
  ProbeSSHHostKey,
  ProbeQuickSSHHostKey,
  RenewFrontendLease,
  RenderSnippet,
  ResetSettings,
  ResizeTerminal,
  RenameRemotePath,
  StartDownload,
  StartSessionLogging,
  StartTunnel,
  StartUpload,
  StopTunnel,
  StopSessionLogging,
  UpdateProfile,
  UpdateSettings,
  UpdateSnippet,
  UpdateTunnel,
  TrustSSHHostKey,
  WriteTerminal,
} from '../../../wailsjs/go/bridge/Desktop'
import { bridge } from '../../../wailsjs/go/models'
import { EventsOff, EventsOn } from '../../../wailsjs/runtime/runtime'
import type {
  AppSettings,
  FrontendLease,
  FileSession,
  Profile,
  ProfileExportResult,
  ProfileImportResult,
  ProfileInput,
  QuickSSHInput,
  QuickSSHProbe,
  Session,
  SessionStateEvent,
  SSHAuthenticationInfo,
  SSHCredentials,
  SSHHostKey,
  SessionLogStatus,
  Snippet,
  SnippetInput,
  SnippetPreview,
  RemoteFile,
  TerminalOutput,
  Transfer,
  TunnelConfig,
  TunnelInput,
  TunnelSnapshot,
} from './types'

export const events = {
  terminalOutput: 'shhh:terminal-output',
  sessionState: 'shhh:session-state',
  closeRequested: 'shhh:close-requested',
  transfer: 'shhh:transfer',
  tunnel: 'shhh:tunnel',
  sessionLog: 'shhh:session-log',
} as const

export const backend = {
  attachFrontend: (nonce: string) => AttachFrontend(nonce) as Promise<FrontendLease>,
  renewFrontendLease: (leaseId: string) => RenewFrontendLease(leaseId) as Promise<FrontendLease>,
  listProfiles: async () => normalizeProfiles(await ListProfiles()),
  createProfile: (profile: ProfileInput) => CreateProfile(profile) as Promise<Profile>,
  updateProfile: (profile: ProfileInput) => UpdateProfile(profile) as Promise<Profile>,
  duplicateProfile: (profileId: string) => DuplicateProfile(profileId) as Promise<Profile>,
  deleteProfile: (profileId: string) => DeleteProfile(profileId),
  importProfiles: async () => {
    const result = await ImportProfiles() as ProfileImportResult
    return {
      ...result,
      imported: normalizeProfiles(result.imported),
      warnings: Array.isArray(result.warnings) ? result.warnings : [],
    }
  },
  exportProfiles: () => ExportProfiles() as Promise<ProfileExportResult>,
  getSettings: () => GetSettings() as Promise<AppSettings>,
  updateSettings: (settings: AppSettings) =>
    UpdateSettings(bridge.SettingsDTO.createFrom(settings)) as Promise<AppSettings>,
  resetSettings: () => ResetSettings() as Promise<AppSettings>,
  listSnippets: async () => normalizeSnippets(await ListSnippets()),
  createSnippet: (input: SnippetInput) => CreateSnippet(input) as Promise<Snippet>,
  updateSnippet: (input: SnippetInput) => UpdateSnippet(input) as Promise<Snippet>,
  deleteSnippet: (snippetId: string) => DeleteSnippet(snippetId),
  renderSnippet: (snippetId: string, values: Record<string, string>) =>
    RenderSnippet(snippetId, values) as Promise<SnippetPreview>,
  listTunnels: async () => ((await ListTunnels()) ?? []) as TunnelConfig[],
  createTunnel: (input: TunnelInput) => CreateTunnel(input) as Promise<TunnelConfig>,
  updateTunnel: (input: TunnelInput) => UpdateTunnel(input) as Promise<TunnelConfig>,
  deleteTunnel: (configId: string) => DeleteTunnel(configId),
  openLocalTerminal: (leaseId: string, profileId: string, columns: number, rows: number) =>
    OpenLocalTerminal(leaseId, profileId, columns, rows) as Promise<Session>,
  probeSSHHostKey: (leaseId: string, profileId: string) =>
    ProbeSSHHostKey(leaseId, profileId) as Promise<SSHHostKey>,
  probeQuickSSHHostKey: async (leaseId: string, input: QuickSSHInput) => {
    const result = await ProbeQuickSSHHostKey(leaseId, input) as QuickSSHProbe
    return { ...result, profile: normalizeProfiles([result.profile])[0] } as QuickSSHProbe
  },
  trustSSHHostKey: (leaseId: string, challengeId: string, permanent: boolean) =>
    TrustSSHHostKey(leaseId, challengeId, permanent),
  inspectSSHAuthentication: (leaseId: string, profileId: string) =>
    InspectSSHAuthentication(leaseId, profileId) as Promise<SSHAuthenticationInfo>,
  inspectQuickSSHAuthentication: (leaseId: string, input: QuickSSHInput) =>
    InspectQuickSSHAuthentication(leaseId, input) as Promise<SSHAuthenticationInfo>,
  openSSHTerminal: (
    leaseId: string,
    profileId: string,
    columns: number,
    rows: number,
    credentials: SSHCredentials,
  ) => OpenSSHTerminal(leaseId, profileId, columns, rows, credentials) as Promise<Session>,
  openQuickSSHTerminal: (
    leaseId: string,
    input: QuickSSHInput,
    columns: number,
    rows: number,
    credentials: SSHCredentials,
  ) => OpenQuickSSHTerminal(leaseId, input, columns, rows, credentials) as Promise<Session>,
  activateTerminal: (leaseId: string, sessionId: string, generation: number) =>
    ActivateTerminal(leaseId, sessionId, generation),
  writeTerminal: (
    leaseId: string,
    sessionId: string,
    generation: number,
    inputSequence: number,
    payload: string,
  ) => WriteTerminal(leaseId, sessionId, generation, inputSequence, payload),
  resizeTerminal: (leaseId: string, sessionId: string, generation: number, columns: number, rows: number) =>
    ResizeTerminal(leaseId, sessionId, generation, columns, rows),
  acknowledgeTerminalOutput: (
    leaseId: string,
    sessionId: string,
    generation: number,
    throughSequence: number,
    bytesConsumed: number,
  ) => AcknowledgeTerminalOutput(leaseId, sessionId, generation, throughSequence, bytesConsumed),
  closeTerminal: (leaseId: string, sessionId: string, generation: number) =>
    CloseTerminal(leaseId, sessionId, generation),
  startSessionLogging: (leaseId: string, sessionId: string, generation: number, timestampLines: boolean) =>
    StartSessionLogging(leaseId, sessionId, generation, timestampLines) as Promise<SessionLogStatus>,
  stopSessionLogging: (leaseId: string, sessionId: string, generation: number) =>
    StopSessionLogging(leaseId, sessionId, generation) as Promise<SessionLogStatus>,
  openSFTP: (leaseId: string, profileId: string, credentials: SSHCredentials) =>
    OpenSFTP(leaseId, profileId, credentials) as Promise<FileSession>,
  listRemoteFiles: async (leaseId: string, sessionId: string, path: string) =>
    ((await ListRemoteFiles(leaseId, sessionId, path)) ?? []) as RemoteFile[],
  createRemoteDirectory: (leaseId: string, sessionId: string, path: string) =>
    CreateRemoteDirectory(leaseId, sessionId, path),
  renameRemotePath: (leaseId: string, sessionId: string, source: string, destination: string) =>
    RenameRemotePath(leaseId, sessionId, source, destination),
  deleteRemotePath: (leaseId: string, sessionId: string, path: string) =>
    DeleteRemotePath(leaseId, sessionId, path),
  chmodRemotePath: (leaseId: string, sessionId: string, path: string, mode: number) =>
    ChmodRemotePath(leaseId, sessionId, path, mode),
  startDownload: (leaseId: string, sessionId: string, path: string) =>
    StartDownload(leaseId, sessionId, path) as Promise<Transfer>,
  startUpload: (leaseId: string, sessionId: string, directory: string) =>
    StartUpload(leaseId, sessionId, directory) as Promise<Transfer>,
  listTransfers: async (leaseId: string) => ((await ListTransfers(leaseId)) ?? []) as Transfer[],
  cancelTransfer: (leaseId: string, transferId: string) => CancelTransfer(leaseId, transferId),
  closeSFTP: (leaseId: string, sessionId: string) => CloseSFTP(leaseId, sessionId),
  startTunnel: (leaseId: string, configId: string, credentials: SSHCredentials) =>
    StartTunnel(leaseId, configId, credentials) as Promise<TunnelSnapshot>,
  stopTunnel: (leaseId: string, configId: string) => StopTunnel(leaseId, configId),
  listTunnelStates: async (leaseId: string) =>
    ((await ListTunnelStates(leaseId)) ?? []) as TunnelSnapshot[],
  confirmApplicationClose: (leaseId: string) => ConfirmApplicationClose(leaseId),
}

function normalizeProfiles(value: unknown): Profile[] {
  if (!Array.isArray(value)) {
    return []
  }
  return value.map((item) => {
    const profile = item as Partial<Profile>
    return {
      ...profile,
      arguments: Array.isArray(profile.arguments) ? profile.arguments : [],
      environment: profile.environment && typeof profile.environment === 'object' ? profile.environment : {},
      tags: Array.isArray(profile.tags) ? profile.tags : [],
      group: profile.group ?? '',
    } as Profile
  })
}

function normalizeSnippets(value: unknown): Snippet[] {
  if (!Array.isArray(value)) {
    return []
  }
  return value.map((item) => {
    const snippet = item as Partial<Snippet>
    return {
      ...snippet,
      folder: snippet.folder ?? '',
      tags: Array.isArray(snippet.tags) ? snippet.tags : [],
      variables: Array.isArray(snippet.variables) ? snippet.variables : [],
    } as Snippet
  })
}

export function onTerminalOutput(callback: (event: TerminalOutput) => void): () => void {
  EventsOn(events.terminalOutput, callback)
  return () => EventsOff(events.terminalOutput)
}

export function onSessionState(callback: (event: SessionStateEvent) => void): () => void {
  EventsOn(events.sessionState, callback)
  return () => EventsOff(events.sessionState)
}

export function onCloseRequested(callback: () => void): () => void {
  EventsOn(events.closeRequested, callback)
  return () => EventsOff(events.closeRequested)
}

export function onTransfer(callback: (event: Transfer) => void): () => void {
  EventsOn(events.transfer, callback)
  return () => EventsOff(events.transfer)
}

export function onTunnel(callback: (event: TunnelSnapshot) => void): () => void {
  EventsOn(events.tunnel, callback)
  return () => EventsOff(events.tunnel)
}

export function onSessionLog(callback: (event: SessionLogStatus) => void): () => void {
  EventsOn(events.sessionLog, callback)
  return () => EventsOff(events.sessionLog)
}
