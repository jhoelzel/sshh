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
  CreateRemotePathFavorite,
  CreateRemoteDirectory,
  CreateSnippet,
  CreateTunnel,
  CreateWorkspaceLayout,
  DeleteProfile,
  DeleteRemotePathFavorite,
  DeleteRemotePath,
  DiscardTransferResume,
  DeleteSnippet,
  DeleteTunnel,
  DeleteWorkspaceLayout,
  DuplicateProfile,
  ExportProfiles,
  ExportTerminalText,
  GetBuildInfo,
  GetNotificationStatus,
  GetSettings,
  ListProfiles,
  ListRemotePathFavorites,
  ListRemoteFiles,
  ListSnippets,
  ListTransferResumes,
  ListTransfers,
  ListTunnelStates,
  ListTunnels,
  ListWorkspaceLayouts,
  InspectSSHAuthentication,
  ImportProfiles,
  InspectQuickSSHAuthentication,
  OpenLocalTerminal,
  OpenQuickSSHTerminal,
  OpenSFTP,
  OpenSSHTerminal,
  ProbeSSHHostKey,
  ProbeQuickSSHHostKey,
  RequestNotificationPermission,
  RenewFrontendLease,
  RenderSnippet,
  ResetSettings,
  ResizeTerminal,
  RenameRemotePath,
  ResumeTransfer,
  SendTestNotification,
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
  UpdateWorkspaceLayout,
  TrustSSHHostKey,
  WriteTerminal,
} from '../../../wailsjs/go/bridge/Desktop'
import { bridge } from '../../../wailsjs/go/models'
import { ClipboardSetText, EventsOn } from '../../../wailsjs/runtime/runtime'
import { asBackendError } from './errors'
import type {
  AppSettings,
  BuildInfo,
  FrontendLease,
  NotificationStatus,
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
  RemotePathFavorite,
  TerminalOutput,
  TerminalTextExportResult,
  Transfer,
  TransferResume,
  TunnelConfig,
  TunnelInput,
  TunnelSnapshot,
  WorkspaceLayout,
  WorkspaceLayoutInput,
} from './types'

export const events = {
  terminalOutput: 'shhh:terminal-output',
  sessionState: 'shhh:session-state',
  closeRequested: 'shhh:close-requested',
  transfer: 'shhh:transfer',
  tunnel: 'shhh:tunnel',
  sessionLog: 'shhh:session-log',
} as const

const rawBackend = {
  attachFrontend: (nonce: string) => AttachFrontend(nonce) as Promise<FrontendLease>,
  renewFrontendLease: (leaseId: string) => RenewFrontendLease(leaseId) as Promise<FrontendLease>,
  getBuildInfo: () => GetBuildInfo() as Promise<BuildInfo>,
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
  copyText: async (text: string) => {
    if (!text) throw new Error('There is no terminal text to copy')
    if (!await ClipboardSetText(text)) throw new Error('The system clipboard rejected the terminal text')
  },
  exportTerminalText: (title: string, text: string) =>
    ExportTerminalText(title, text) as Promise<TerminalTextExportResult>,
  listRemotePathFavorites: async () =>
    ((await ListRemotePathFavorites()) ?? []) as RemotePathFavorite[],
  createRemotePathFavorite: (profileId: string, path: string) =>
    CreateRemotePathFavorite(profileId, path) as Promise<RemotePathFavorite>,
  deleteRemotePathFavorite: (favoriteId: string) => DeleteRemotePathFavorite(favoriteId),
  getSettings: () => GetSettings() as Promise<AppSettings>,
  getNotificationStatus: () => GetNotificationStatus() as Promise<NotificationStatus>,
  requestNotificationPermission: () => RequestNotificationPermission() as Promise<NotificationStatus>,
  sendTestNotification: () => SendTestNotification(),
  updateSettings: (settings: AppSettings) =>
    UpdateSettings(bridge.SettingsDTO.createFrom(settings)) as Promise<AppSettings>,
  resetSettings: () => ResetSettings() as Promise<AppSettings>,
  listSnippets: async () => normalizeSnippets(await ListSnippets()),
  createSnippet: (input: SnippetInput) => CreateSnippet(input) as Promise<Snippet>,
  updateSnippet: (input: SnippetInput) => UpdateSnippet(input) as Promise<Snippet>,
  deleteSnippet: (snippetId: string) => DeleteSnippet(snippetId),
  renderSnippet: (snippetId: string, values: Record<string, string>) =>
    RenderSnippet(snippetId, values) as Promise<SnippetPreview>,
  listWorkspaceLayouts: async () => normalizeWorkspaceLayouts(await ListWorkspaceLayouts()),
  createWorkspaceLayout: (input: WorkspaceLayoutInput) =>
    CreateWorkspaceLayout(bridge.WorkspaceLayoutInputDTO.createFrom(input)) as Promise<WorkspaceLayout>,
  updateWorkspaceLayout: (input: WorkspaceLayoutInput) =>
    UpdateWorkspaceLayout(bridge.WorkspaceLayoutInputDTO.createFrom(input)) as Promise<WorkspaceLayout>,
  deleteWorkspaceLayout: (layoutId: string) => DeleteWorkspaceLayout(layoutId),
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
  listTransferResumes: async (leaseId: string, sessionId: string) =>
    ((await ListTransferResumes(leaseId, sessionId)) ?? []) as TransferResume[],
  resumeTransfer: (leaseId: string, sessionId: string, resumeId: string) =>
    ResumeTransfer(leaseId, sessionId, resumeId) as Promise<Transfer>,
  discardTransferResume: (leaseId: string, sessionId: string, resumeId: string) =>
    DiscardTransferResume(leaseId, sessionId, resumeId),
  cancelTransfer: (leaseId: string, transferId: string) => CancelTransfer(leaseId, transferId),
  closeSFTP: (leaseId: string, sessionId: string) => CloseSFTP(leaseId, sessionId),
  startTunnel: (leaseId: string, configId: string, credentials: SSHCredentials) =>
    StartTunnel(leaseId, configId, credentials) as Promise<TunnelSnapshot>,
  stopTunnel: (leaseId: string, configId: string) => StopTunnel(leaseId, configId),
  listTunnelStates: async (leaseId: string) =>
    ((await ListTunnelStates(leaseId)) ?? []) as TunnelSnapshot[],
  confirmApplicationClose: (leaseId: string) => ConfirmApplicationClose(leaseId),
}

export const backend = withBackendErrors(rawBackend)

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

function normalizeWorkspaceLayouts(value: unknown): WorkspaceLayout[] {
  if (!Array.isArray(value)) {
    return []
  }
  return value.map((item) => {
    const layout = item as Partial<WorkspaceLayout>
    return {
      ...layout,
      tabs: Array.isArray(layout.tabs) ? layout.tabs : [],
      activeTab: Number.isInteger(layout.activeTab) ? layout.activeTab : 0,
    } as WorkspaceLayout
  })
}

export function onTerminalOutput(callback: (event: TerminalOutput) => void): () => void {
  return EventsOn(events.terminalOutput, callback)
}

export function onSessionState(callback: (event: SessionStateEvent) => void): () => void {
  return EventsOn(events.sessionState, callback)
}

export function onCloseRequested(callback: () => void): () => void {
  return EventsOn(events.closeRequested, callback)
}

export function onTransfer(callback: (event: Transfer) => void): () => void {
  return EventsOn(events.transfer, callback)
}

export function onTunnel(callback: (event: TunnelSnapshot) => void): () => void {
  return EventsOn(events.tunnel, callback)
}

export function onSessionLog(callback: (event: SessionLogStatus) => void): () => void {
  return EventsOn(events.sessionLog, callback)
}

function withBackendErrors<T extends object>(client: T): T {
  const wrappers = new Map<PropertyKey, unknown>()
  return new Proxy(client, {
    get(target, property, receiver) {
      const value = Reflect.get(target, property, receiver)
      if (typeof value !== 'function') {
        return value
      }
      if (wrappers.has(property)) {
        return wrappers.get(property)
      }
      const wrapper = (...args: unknown[]) => {
        try {
          return Promise.resolve(Reflect.apply(value, target, args))
            .catch((cause: unknown) => {
              throw asBackendError(cause)
            })
        } catch (cause) {
          return Promise.reject(asBackendError(cause))
        }
      }
      wrappers.set(property, wrapper)
      return wrapper
    },
  })
}
