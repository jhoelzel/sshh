import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Braces,
  ChevronDown,
  ChevronUp,
  CircleAlert,
  Command,
  Copy,
  FileText,
  FileDown,
  FileUp,
  FolderOpen,
  LayoutPanelTop,
  Laptop,
  LoaderCircle,
  Network,
  Pencil,
  Plus,
  Search,
  Settings2,
  Star,
  TerminalSquare,
  Zap,
  X,
} from 'lucide-react'
import { CommandPalette, type PaletteCommand } from '../feature/commands/CommandPalette'
import { FileBrowser } from '../feature/files/FileBrowser'
import { canonicalRemotePath } from '../feature/files/remotePath'
import { ProfileEditor } from '../feature/profile/ProfileEditor'
import { ProfileExchangeDialog } from '../feature/profile/ProfileExchangeDialog'
import { QuickConnectDialog } from '../feature/ssh/QuickConnectDialog'
import { SSHCredentialsDialog, SSHTrustDialog } from '../feature/ssh/SSHConnectDialog'
import { SettingsWorkspace } from '../feature/settings/SettingsWorkspace'
import { SnippetWorkspace } from '../feature/snippets/SnippetWorkspace'
import { LoggingDialog } from '../feature/terminal/LoggingDialog'
import { copyVisibleText, exportSelectedText } from '../feature/terminal/terminalActions'
import type { TerminalController } from '../feature/terminal/TerminalController'
import { TerminalPane } from '../feature/terminal/TerminalPane'
import { TunnelWorkspace } from '../feature/tunnels/TunnelWorkspace'
import { WorkspaceLayoutWorkspace } from '../feature/workspaces/WorkspaceLayoutWorkspace'
import { backend, onCloseRequested, onSessionLog, onSessionState, onTerminalOutput, onTransfer, onTunnel } from '../lib/bridge/client'
import { createDisconnectedTabs } from './workspaces'
import type {
  AppSettings,
  FileSession,
  FrontendLease,
  Profile,
  ProfileExchangeResult,
  ProfileInput,
  QuickSSHInput,
  RemoteFile,
  RemotePathFavorite,
  Session,
  SessionLogStatus,
  SessionState,
  SessionStateEvent,
  SSHAuthenticationInfo,
  SSHCredentials,
  SSHHostKey,
  Snippet,
  SnippetInput,
  SnippetPreview,
  Transfer,
  TunnelConfig,
  TunnelInput,
  TunnelSnapshot,
  WorkspaceLayout,
  WorkspaceLayoutInput,
} from '../lib/bridge/types'

const frontendNonce = crypto.randomUUID()
const initialColumns = 100
const initialRows = 30
const isMacOS = navigator.userAgent.includes('Macintosh')
const shortcutPrefix = isMacOS ? 'Cmd Shift' : 'Ctrl Shift'

type TerminalTabState = SessionState | 'disconnected'

interface TabModel {
  id: string
  profileId: string
  endpoint: string
  session?: Session
  controller?: TerminalController
  title: string
  state: TerminalTabState
  exitSummary?: string
  attention: boolean
  hasSelection: boolean
}

type Confirmation =
  | { kind: 'close-tab'; tabId: string }
  | { kind: 'close-application' }
  | { kind: 'delete-profile'; profileId: string }
  | { kind: 'restore-layout'; layoutId: string }

interface ProfileEditorState {
  profile?: Profile
}

type SSHAction = { kind: 'terminal'; replaceTabId?: string } | { kind: 'files' } | { kind: 'tunnel'; configId: string }

type SSHPrompt =
  | { kind: 'trust'; action: SSHAction; profile: Profile; hostKey: SSHHostKey; quick?: QuickSSHInput }
  | { kind: 'credentials'; action: SSHAction; profile: Profile; authentication: SSHAuthenticationInfo; quick?: QuickSSHInput }

export function App() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [lease, setLease] = useState<FrontendLease>()
  const [tabs, setTabs] = useState<TabModel[]>([])
  const [workspaceMode, setWorkspaceMode] = useState<'terminals' | 'files' | 'tunnels' | 'snippets' | 'layouts' | 'settings'>('terminals')
  const [activeId, setActiveId] = useState<string>()
  const [openingProfile, setOpeningProfile] = useState<string>()
  const [profileEditor, setProfileEditor] = useState<ProfileEditorState>()
  const [profileExchange, setProfileExchange] = useState<ProfileExchangeResult>()
  const [profileExchangeAction, setProfileExchangeAction] = useState<'import' | 'export'>()
  const [quickConnectOpen, setQuickConnectOpen] = useState(false)
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false)
  const [sshPrompt, setSSHPrompt] = useState<SSHPrompt>()
  const [profileFilter, setProfileFilter] = useState('')
  const [confirmation, setConfirmation] = useState<Confirmation>()
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [fileSession, setFileSession] = useState<FileSession>()
  const [fileProfile, setFileProfile] = useState<Profile>()
  const [remotePath, setRemotePath] = useState('')
  const [remoteFiles, setRemoteFiles] = useState<RemoteFile[]>([])
  const [remotePathFavorites, setRemotePathFavorites] = useState<RemotePathFavorite[]>([])
  const [fileLoading, setFileLoading] = useState(false)
  const [transfers, setTransfers] = useState<Transfer[]>([])
  const [tunnelConfigs, setTunnelConfigs] = useState<TunnelConfig[]>([])
  const [tunnelSnapshots, setTunnelSnapshots] = useState<TunnelSnapshot[]>([])
  const [snippets, setSnippets] = useState<Snippet[]>([])
  const [workspaceLayouts, setWorkspaceLayouts] = useState<WorkspaceLayout[]>([])
  const [sessionLogs, setSessionLogs] = useState<SessionLogStatus[]>([])
  const [settings, setSettings] = useState<AppSettings>()
  const [loggingSessionId, setLoggingSessionId] = useState<string>()
  const [error, setError] = useState<string>()
  const [notice, setNotice] = useState<string>()
  const controllers = useRef(new Map<string, TerminalController>())
  const autoStartAttempted = useRef(new Set<string>())

  const activeTab = useMemo(() => tabs.find((tab) => tab.id === activeId), [activeId, tabs])
  const activeProfile = useMemo(
    () => profiles.find((profile) => profile.id === activeTab?.profileId),
    [activeTab?.profileId, profiles],
  )
  const activeLog = useMemo(
    () => sessionLogs.find((status) => status.sessionId === activeTab?.session?.id && status.active),
    [activeTab?.session?.id, sessionLogs],
  )
  const snippetTargets = useMemo(
    () => tabs.flatMap((tab) => tab.state === 'running' && tab.session ? [{
      id: tab.session.id, title: tab.title, active: tab.id === activeId,
    }] : []),
    [activeId, tabs],
  )
  const localProfiles = useMemo(
    () => profiles.filter((item) => item.protocol === 'local' && item.connectable),
    [profiles],
  )
  const visibleProfiles = useMemo(() => filterAndSortProfiles(profiles, profileFilter), [profileFilter, profiles])
  const workspaceSnapshot = useMemo(
    () => captureWorkspace(tabs, profiles, activeId),
    [activeId, profiles, tabs],
  )
  const fileFavorites = useMemo(
    () => remotePathFavorites.filter((favorite) => favorite.profileId === fileProfile?.id),
    [fileProfile?.id, remotePathFavorites],
  )
  const runningCount = tabs.filter((tab) => isLive(tab.state)).length
  const activeTransferCount = transfers.filter((transfer) => transfer.state === 'queued' || transfer.state === 'running').length
  const activeTunnelCount = tunnelSnapshots.filter((snapshot) => isLiveTunnel(snapshot.state)).length
  const activityCount = runningCount + activeTransferCount + activeTunnelCount + (fileSession ? 1 : 0)

  const reportError = useCallback((cause: unknown) => {
    setNotice(undefined)
    setError(cause instanceof Error ? cause.message : String(cause))
  }, [])

  const reportNotice = useCallback((message: string) => {
    setError(undefined)
    setNotice(message)
  }, [])

  useEffect(() => {
    if (!notice) return
    const timer = window.setTimeout(() => setNotice(undefined), 3_000)
    return () => window.clearTimeout(timer)
  }, [notice])

  useEffect(() => {
    let cancelled = false
    void Promise.all([
      backend.attachFrontend(frontendNonce), backend.listProfiles(), backend.listTunnels(),
      backend.listSnippets(), backend.listWorkspaceLayouts(), backend.listRemotePathFavorites(), backend.getSettings(),
    ]).then(([attachedLease, loadedProfiles, loadedTunnels, loadedSnippets, loadedLayouts, loadedPathFavorites, loadedSettings]) => {
        if (!cancelled) {
          setLease(attachedLease)
          setProfiles(loadedProfiles)
          setTunnelConfigs(loadedTunnels)
          setSnippets(loadedSnippets)
          setWorkspaceLayouts(loadedLayouts)
          setRemotePathFavorites(loadedPathFavorites)
          setSettings(loadedSettings)
        }
    })
      .catch(reportError)
    return () => {
      cancelled = true
    }
  }, [reportError])

  useEffect(() => {
    if (!lease) {
      return
    }
    const disposeOutput = onTerminalOutput((event) => {
      if (event.leaseId === lease.id) {
        controllers.current.get(event.sessionId)?.acceptOutput(event)
      }
    })
    const disposeState = onSessionState((event) => {
      if (event.leaseId !== lease.id) {
        return
      }
      setTabs((current) => updateTabState(current, event))
    })
    const disposeTransfer = onTransfer((event) => {
      if (event.leaseId === lease.id) {
        setTransfers((current) => upsertTransfer(current, event))
      }
    })
    const disposeTunnel = onTunnel((event) => {
      if (event.leaseId === lease.id) {
        setTunnelSnapshots((current) => upsertTunnelSnapshot(current, event))
      }
    })
    const disposeLog = onSessionLog((event) => {
      if (event.leaseId === lease.id) {
        setSessionLogs((current) => upsertSessionLog(current, event))
        if (event.message) reportError(event.message)
      }
    })
    const disposeClose = onCloseRequested(() => setConfirmation({ kind: 'close-application' }))
    return () => {
      disposeOutput()
      disposeState()
      disposeTransfer()
      disposeTunnel()
      disposeLog()
      disposeClose()
    }
  }, [lease, reportError])

  useEffect(() => {
    if (!lease || activityCount === 0) {
      return
    }
    const timer = window.setInterval(() => {
      void backend.renewFrontendLease(lease.id).catch(reportError)
    }, 5_000)
    return () => window.clearInterval(timer)
  }, [activityCount, lease, reportError])

  useEffect(() => {
    if (!lease) {
      return
    }
    void Promise.all([backend.listTransfers(lease.id), backend.listTunnelStates(lease.id)])
      .then(([loadedTransfers, loadedTunnels]) => {
        setTransfers(loadedTransfers)
        setTunnelSnapshots(loadedTunnels)
      })
      .catch(reportError)
  }, [lease, reportError])

  useEffect(() => {
    if (!lease) return
    for (const config of tunnelConfigs) {
      const live = tunnelSnapshots.some((snapshot) => snapshot.configId === config.id && isLiveTunnel(snapshot.state))
      if (!config.autoStart || live || autoStartAttempted.current.has(config.id)) continue
      autoStartAttempted.current.add(config.id)
      const selected = profiles.find((profile) => profile.id === config.profileId)
      if (!selected) continue
      void (async () => {
        const hostKey = await backend.probeSSHHostKey(lease.id, selected.id)
        if (hostKey.status !== 'known') {
          throw new Error(`${config.name} requires host-key approval before auto-start.`)
        }
        const authentication = await backend.inspectSSHAuthentication(lease.id, selected.id)
        if (authentication.secret !== 'none') {
          throw new Error(`${config.name} requires credentials and must be started manually.`)
        }
        const snapshot = await backend.startTunnel(lease.id, config.id, { password: '', passphrase: '' })
        setTunnelSnapshots((current) => upsertTunnelSnapshot(current, snapshot))
      })().catch(reportError)
    }
  }, [lease, profiles, reportError, tunnelConfigs, tunnelSnapshots])

  useEffect(() => {
    const current = controllers.current
    return () => {
      for (const controller of current.values()) {
        controller.dispose()
      }
      current.clear()
    }
  }, [])

  const removeTab = useCallback((tabId: string) => {
    controllers.current.get(tabId)?.dispose()
    controllers.current.delete(tabId)
    setSessionLogs((current) => current.filter((status) => status.sessionId !== tabId))
    setLoggingSessionId((current) => current === tabId ? undefined : current)
    setTabs((current) => {
      const index = current.findIndex((tab) => tab.id === tabId)
      const next = current.filter((tab) => tab.id !== tabId)
      setActiveId((active) => {
        if (active !== tabId) {
          return active
        }
        return next[Math.min(index, next.length - 1)]?.id
      })
      return next
    })
  }, [])

  const saveProfile = useCallback(async (input: ProfileInput) => {
    const saved = input.id ? await backend.updateProfile(input) : await backend.createProfile(input)
    setProfiles((current) => {
      const exists = current.some((item) => item.id === saved.id)
      return exists ? current.map((item) => (item.id === saved.id ? saved : item)) : [...current, saved]
    })
    setProfileEditor(undefined)
  }, [])

  const duplicateProfile = useCallback(async (profileId: string) => {
    const duplicated = await backend.duplicateProfile(profileId)
    setProfiles((current) => [...current, duplicated])
    setProfileEditor({ profile: duplicated })
  }, [])

  const importProfiles = useCallback(async () => {
    setProfileExchangeAction('import')
    setError(undefined)
    try {
      const result = await backend.importProfiles()
      if (!result.cancelled) {
        setProfiles((current) => [...current, ...result.imported])
        setProfileExchange({ kind: 'import', result })
      }
    } catch (cause) {
      reportError(cause)
    } finally {
      setProfileExchangeAction(undefined)
    }
  }, [reportError])

  const exportProfiles = useCallback(async () => {
    setProfileExchangeAction('export')
    setError(undefined)
    try {
      const result = await backend.exportProfiles()
      if (!result.cancelled) {
        setProfileExchange({ kind: 'export', result })
      }
    } catch (cause) {
      reportError(cause)
    } finally {
      setProfileExchangeAction(undefined)
    }
  }, [reportError])

  const openTerminalSession = useCallback(
    async (
      selected: Profile,
      credentials: SSHCredentials = { password: '', passphrase: '' },
      quick?: QuickSSHInput,
      replaceTabId?: string,
    ) => {
      if (!lease || !settings) throw new Error('Terminal backend is not ready')
      if (!selected.connectable) throw new Error('Selected profile cannot open a terminal')
      let opened: { session: Session; controller: TerminalController } | undefined
      let replaced: { tab: TabModel; index: number } | undefined
      try {
        const { TerminalController: Controller } = await import('../feature/terminal/TerminalController')
        const session = selected.protocol === 'local'
          ? await backend.openLocalTerminal(lease.id, selected.id, initialColumns, initialRows)
          : quick
            ? await backend.openQuickSSHTerminal(lease.id, quick, initialColumns, initialRows, credentials)
            : await backend.openSSHTerminal(lease.id, selected.id, initialColumns, initialRows, credentials)
        const controller = new Controller(session, settings.terminal, {
          onTitle: (title) =>
            setTabs((current) => current.map((tab) => (tab.id === session.id ? { ...tab, title } : tab))),
          onBell: () =>
            setTabs((current) =>
              current.map((tab) => (tab.id === session.id ? { ...tab, attention: true } : tab)),
            ),
          onError: reportError,
          onSearchRequested: () => setSearchOpen(true),
          onSelectionChange: (hasSelection) =>
            setTabs((current) => current.map((tab) => tab.id === session.id ? { ...tab, hasSelection } : tab)),
        })
        opened = { session, controller }
        controllers.current.set(session.id, controller)
        const liveTab: TabModel = {
          id: session.id,
          profileId: selected.id,
          endpoint: selected.endpoint,
          session,
          controller,
          title: session.title,
          state: session.state,
          attention: false,
          hasSelection: false,
        }
        setTabs((current) => {
          if (!replaceTabId) return [...current, liveTab]
          const index = current.findIndex((tab) => tab.id === replaceTabId)
          if (index < 0) return [...current, liveTab]
          replaced = { tab: current[index], index }
          const next = [...current]
          next[index] = liveTab
          return next
        })
        setActiveId(session.id)
        await controller.ready()
        await backend.activateTerminal(lease.id, session.id, session.generation)
      } catch (cause) {
        let failure = cause
        if (opened) {
          try {
            await backend.closeTerminal(lease.id, opened.session.id, opened.session.generation)
          } catch (cleanupCause) {
            failure = new Error(
              `${errorMessage(cause)} Terminal cleanup also failed: ${errorMessage(cleanupCause)}`,
            )
          }
          removeTab(opened.session.id)
          if (replaced) {
            const saved = replaced
            setTabs((current) => {
              const next = [...current]
              next.splice(Math.min(saved.index, next.length), 0, saved.tab)
              return next
            })
            setActiveId(saved.tab.id)
          }
        }
        throw failure
      }
    },
    [lease, removeTab, reportError, settings],
  )

  const openFileWorkspace = useCallback(
    async (selected: Profile, credentials: SSHCredentials = { password: '', passphrase: '' }) => {
      if (!lease) {
        throw new Error('Backend is not attached')
      }
      const opened = await backend.openSFTP(lease.id, selected.id, credentials)
      try {
        const root = canonicalRemotePath(opened.root)
        const entries = await backend.listRemoteFiles(lease.id, opened.id, root)
        if (fileSession) {
          await backend.closeSFTP(lease.id, fileSession.id)
        }
        setFileSession(opened)
        setFileProfile(selected)
        setRemotePath(root)
        setRemoteFiles(entries)
        setWorkspaceMode('files')
      } catch (cause) {
        await backend.closeSFTP(lease.id, opened.id).catch(() => undefined)
        throw cause
      }
    },
    [fileSession, lease],
  )

  const performSSHAction = useCallback(
    async (selected: Profile, action: SSHAction, credentials?: SSHCredentials, quick?: QuickSSHInput) => {
      if (action.kind === 'terminal') {
        await openTerminalSession(selected, credentials, quick, action.replaceTabId)
        setWorkspaceMode('terminals')
      } else if (action.kind === 'files') {
        if (quick) throw new Error('Quick connections support terminals only')
        await openFileWorkspace(selected, credentials)
      } else {
        if (quick) throw new Error('Quick connections cannot start saved tunnels')
        if (!lease) throw new Error('Backend is not attached')
        const snapshot = await backend.startTunnel(
          lease.id,
          action.configId,
          credentials ?? { password: '', passphrase: '' },
        )
        setTunnelSnapshots((current) => upsertTunnelSnapshot(current, snapshot))
        setWorkspaceMode('tunnels')
      }
    },
    [lease, openFileWorkspace, openTerminalSession],
  )

  const inspectSSHAndConnect = useCallback(
    async (selected: Profile, action: SSHAction, quick?: QuickSSHInput): Promise<boolean> => {
      if (!lease) {
        throw new Error('Backend is not attached')
      }
      const authentication = quick
        ? await backend.inspectQuickSSHAuthentication(lease.id, quick)
        : await backend.inspectSSHAuthentication(lease.id, selected.id)
      if (authentication.secret === 'none') {
        await performSSHAction(selected, action, undefined, quick)
        return true
      }
      setSSHPrompt({ kind: 'credentials', action, profile: selected, authentication, quick })
      return false
    },
    [lease, performSSHAction],
  )

  const startProfileAction = useCallback(
    async (selected: Profile, action: SSHAction) => {
      if (!lease || !selected.connectable || openingProfile) {
        return
      }
      setOpeningProfile(selected.id)
      setError(undefined)
      try {
        if (selected.protocol === 'local') {
          if (action.kind !== 'terminal') {
            throw new Error('Local profiles do not support SFTP')
          }
          await openTerminalSession(selected, undefined, undefined, action.replaceTabId)
          setWorkspaceMode('terminals')
          setOpeningProfile(undefined)
          return
        }
        const hostKey = await backend.probeSSHHostKey(lease.id, selected.id)
        if (hostKey.status === 'changed') {
          throw new Error(`Host key changed for ${hostKey.address}. Connection blocked (${hostKey.fingerprint}).`)
        }
        if (hostKey.status === 'unknown') {
          setSSHPrompt({ kind: 'trust', action, profile: selected, hostKey })
          return
        }
        if (await inspectSSHAndConnect(selected, action)) {
          setOpeningProfile(undefined)
        }
      } catch (cause) {
        setOpeningProfile(undefined)
        reportError(cause)
      }
    },
    [inspectSSHAndConnect, lease, openTerminalSession, openingProfile, reportError],
  )

  const connectProfile = useCallback(
    (selected: Profile) => startProfileAction(selected, { kind: 'terminal' }),
    [startProfileAction],
  )

  const connectRestoredTab = useCallback((tab: TabModel) => {
    const selected = profiles.find((profile) => profile.id === tab.profileId)
    if (!selected) {
      reportError(new Error('The profile used by this layout is no longer available'))
      return
    }
    void startProfileAction(selected, { kind: 'terminal', replaceTabId: tab.id })
  }, [profiles, reportError, startProfileAction])

  const browseProfile = useCallback(
    (selected: Profile) => startProfileAction(selected, { kind: 'files' }),
    [startProfileAction],
  )

  const startQuickConnect = useCallback(async (input: QuickSSHInput) => {
    if (!lease) throw new Error('Backend is not attached')
    if (openingProfile) throw new Error('Another connection is already opening')
    setOpeningProfile('quick-ssh')
    setError(undefined)
    try {
      const probe = await backend.probeQuickSSHHostKey(lease.id, input)
      const selected = probe.profile
      const hostKey = probe.hostKey
      setOpeningProfile(selected.id)
      if (hostKey.status === 'changed') {
        throw new Error(`Host key changed for ${hostKey.address}. Connection blocked (${hostKey.fingerprint}).`)
      }
      if (hostKey.status === 'unknown') {
        setSSHPrompt({ kind: 'trust', action: { kind: 'terminal' }, profile: selected, hostKey, quick: input })
        setQuickConnectOpen(false)
        return
      }
      const connected = await inspectSSHAndConnect(selected, { kind: 'terminal' }, input)
      setQuickConnectOpen(false)
      if (connected) setOpeningProfile(undefined)
    } catch (cause) {
      setOpeningProfile(undefined)
      throw cause
    }
  }, [inspectSSHAndConnect, lease, openingProfile])

  const trustSSHHost = useCallback(
    async (permanent: boolean) => {
      if (!lease || sshPrompt?.kind !== 'trust') {
        throw new Error('Host-key trust request is no longer active')
      }
      const selected = sshPrompt.profile
      await backend.trustSSHHostKey(lease.id, sshPrompt.hostKey.challengeId, permanent)
      if (await inspectSSHAndConnect(selected, sshPrompt.action, sshPrompt.quick)) {
        setSSHPrompt(undefined)
        setOpeningProfile(undefined)
      }
    },
    [inspectSSHAndConnect, lease, sshPrompt],
  )

  const connectWithSecret = useCallback(
    async (secret: string) => {
      if (sshPrompt?.kind !== 'credentials') {
        throw new Error('Credential request is no longer active')
      }
      const credentials: SSHCredentials = sshPrompt.authentication.secret === 'passphrase'
        ? { password: '', passphrase: secret }
        : { password: secret, passphrase: '' }
      await performSSHAction(sshPrompt.profile, sshPrompt.action, credentials, sshPrompt.quick)
      setSSHPrompt(undefined)
      setOpeningProfile(undefined)
    },
    [performSSHAction, sshPrompt],
  )

  const cancelSSHConnection = useCallback(() => {
    setSSHPrompt(undefined)
    setOpeningProfile(undefined)
  }, [])

  const navigateRemote = useCallback(
    async (targetPath: string) => {
      if (!lease || !fileSession) {
        throw new Error('No remote file session is open')
      }
      setFileLoading(true)
      try {
        const canonicalPath = canonicalRemotePath(targetPath)
        const entries = await backend.listRemoteFiles(lease.id, fileSession.id, canonicalPath)
        setRemotePath(canonicalPath)
        setRemoteFiles(entries)
      } finally {
        setFileLoading(false)
      }
    },
    [fileSession, lease],
  )

  const refreshRemote = useCallback(async () => {
    await navigateRemote(remotePath)
  }, [navigateRemote, remotePath])

  const createRemoteDirectory = useCallback(
    async (targetPath: string) => {
      if (!lease || !fileSession) throw new Error('No remote file session is open')
      await backend.createRemoteDirectory(lease.id, fileSession.id, targetPath)
    },
    [fileSession, lease],
  )

  const renameRemotePath = useCallback(
    async (source: string, destination: string) => {
      if (!lease || !fileSession) throw new Error('No remote file session is open')
      await backend.renameRemotePath(lease.id, fileSession.id, source, destination)
    },
    [fileSession, lease],
  )

  const deleteRemotePath = useCallback(
    async (targetPath: string) => {
      if (!lease || !fileSession) throw new Error('No remote file session is open')
      await backend.deleteRemotePath(lease.id, fileSession.id, targetPath)
    },
    [fileSession, lease],
  )

  const chmodRemotePath = useCallback(
    async (targetPath: string, mode: number) => {
      if (!lease || !fileSession) throw new Error('No remote file session is open')
      await backend.chmodRemotePath(lease.id, fileSession.id, targetPath, mode)
    },
    [fileSession, lease],
  )

  const startDownload = useCallback(
    async (targetPath: string) => {
      if (!lease || !fileSession) throw new Error('No remote file session is open')
      const transfer = await backend.startDownload(lease.id, fileSession.id, targetPath)
      if (transfer.id) setTransfers((current) => upsertTransfer(current, transfer))
    },
    [fileSession, lease],
  )

  const startUpload = useCallback(async () => {
    if (!lease || !fileSession) throw new Error('No remote file session is open')
    const transfer = await backend.startUpload(lease.id, fileSession.id, remotePath)
    if (transfer.id) setTransfers((current) => upsertTransfer(current, transfer))
  }, [fileSession, lease, remotePath])

  const cancelTransfer = useCallback(
    async (transferId: string) => {
      if (!lease) throw new Error('Backend is not attached')
      await backend.cancelTransfer(lease.id, transferId)
    },
    [lease],
  )

  const createRemotePathFavorite = useCallback(async (targetPath: string) => {
    if (!fileProfile) throw new Error('No remote file profile is open')
    const created = await backend.createRemotePathFavorite(fileProfile.id, canonicalRemotePath(targetPath))
    setRemotePathFavorites((current) => [...current, created])
    reportNotice('Remote path added to favorites')
  }, [fileProfile, reportNotice])

  const deleteRemotePathFavorite = useCallback(async (favoriteId: string) => {
    await backend.deleteRemotePathFavorite(favoriteId)
    setRemotePathFavorites((current) => current.filter((favorite) => favorite.id !== favoriteId))
    reportNotice('Remote path removed from favorites')
  }, [reportNotice])

  const closeFileWorkspace = useCallback(async () => {
    if (!lease || !fileSession) return
    await backend.closeSFTP(lease.id, fileSession.id)
    setFileSession(undefined)
    setFileProfile(undefined)
    setRemoteFiles([])
    setRemotePath('')
    setWorkspaceMode('terminals')
  }, [fileSession, lease])

  const createTunnel = useCallback(async (input: TunnelInput) => {
    const created = await backend.createTunnel(input)
    setTunnelConfigs((current) => [...current, created])
  }, [])

  const updateTunnel = useCallback(async (input: TunnelInput) => {
    const updated = await backend.updateTunnel(input)
    setTunnelConfigs((current) => current.map((config) => config.id === updated.id ? updated : config))
  }, [])

  const deleteTunnel = useCallback(async (configId: string) => {
    await backend.deleteTunnel(configId)
    setTunnelConfigs((current) => current.filter((config) => config.id !== configId))
    setTunnelSnapshots((current) => current.filter((snapshot) => snapshot.configId !== configId))
  }, [])

  const startTunnel = useCallback(async (config: TunnelConfig) => {
    const selected = profiles.find((profile) => profile.id === config.profileId)
    if (!selected) throw new Error('Tunnel SSH profile is unavailable')
    await startProfileAction(selected, { kind: 'tunnel', configId: config.id })
  }, [profiles, startProfileAction])

  const stopTunnel = useCallback(async (config: TunnelConfig) => {
    if (!lease) throw new Error('Backend is not attached')
    await backend.stopTunnel(lease.id, config.id)
  }, [lease])

  const restartTunnel = useCallback(async (config: TunnelConfig) => {
    if (!lease) throw new Error('Backend is not attached')
    await backend.stopTunnel(lease.id, config.id)
    await startTunnel(config)
  }, [lease, startTunnel])

  const createSnippet = useCallback(async (input: SnippetInput) => {
    const created = await backend.createSnippet(input)
    setSnippets((current) => sortSnippets([...current, created]))
  }, [])

  const updateSnippet = useCallback(async (input: SnippetInput) => {
    const updated = await backend.updateSnippet(input)
    setSnippets((current) => sortSnippets(current.map((snippet) => snippet.id === updated.id ? updated : snippet)))
  }, [])

  const deleteSnippet = useCallback(async (snippetId: string) => {
    await backend.deleteSnippet(snippetId)
    setSnippets((current) => current.filter((snippet) => snippet.id !== snippetId))
  }, [])

  const createWorkspaceLayout = useCallback(async (name: string) => {
    if (workspaceSnapshot.tabs.length === 0) throw new Error('No saved-profile terminal tabs are open')
    const created = await backend.createWorkspaceLayout({ id: '', name, ...workspaceSnapshot })
    setWorkspaceLayouts((current) => sortWorkspaceLayouts([...current, created]))
  }, [workspaceSnapshot])

  const renameWorkspaceLayout = useCallback(async (layout: WorkspaceLayout, name: string) => {
    const updated = await backend.updateWorkspaceLayout(layoutInput(layout, { name }))
    setWorkspaceLayouts((current) => sortWorkspaceLayouts(current.map((item) => item.id === updated.id ? updated : item)))
  }, [])

  const replaceWorkspaceLayout = useCallback(async (layout: WorkspaceLayout) => {
    if (workspaceSnapshot.tabs.length === 0) throw new Error('No saved-profile terminal tabs are open')
    const updated = await backend.updateWorkspaceLayout(layoutInput(layout, workspaceSnapshot))
    setWorkspaceLayouts((current) => sortWorkspaceLayouts(current.map((item) => item.id === updated.id ? updated : item)))
  }, [workspaceSnapshot])

  const deleteWorkspaceLayout = useCallback(async (layout: WorkspaceLayout) => {
    await backend.deleteWorkspaceLayout(layout.id)
    setWorkspaceLayouts((current) => current.filter((item) => item.id !== layout.id))
  }, [])

  const renderSnippet = useCallback(
    (snippetId: string, values: Record<string, string>): Promise<SnippetPreview> => backend.renderSnippet(snippetId, values),
    [],
  )

  const applySettings = useCallback((saved: AppSettings) => {
    setSettings(saved)
    for (const controller of controllers.current.values()) {
      controller.applySettings(saved.terminal)
    }
  }, [])

  const saveSettings = useCallback(async (draft: AppSettings) => {
    const saved = await backend.updateSettings(draft)
    applySettings(saved)
    return saved
  }, [applySettings])

  const resetSettings = useCallback(async () => {
    const reset = await backend.resetSettings()
    applySettings(reset)
    return reset
  }, [applySettings])

  const executeSnippet = useCallback(async (text: string, targetIds: string[], submit: boolean) => {
    const targets = targetIds.map((id) => tabs.find((tab) => tab.session?.id === id))
    if (targets.some((tab) => !tab || tab.state !== 'running' || !tab.controller)) {
      throw new Error('One or more target terminals are no longer running')
    }
    await Promise.all(targets.map((tab) => tab!.controller!.sendText(text, submit)))
    if (targetIds.length === 1) setActiveId(targets[0]!.id)
    setWorkspaceMode('terminals')
  }, [tabs])

  const startSessionLogging = useCallback(async (timestampLines: boolean) => {
    const tab = tabs.find((item) => item.session?.id === loggingSessionId)
    if (!lease || !tab?.session || tab.state !== 'running') throw new Error('Terminal is no longer running')
    const status = await backend.startSessionLogging(
      lease.id, tab.session.id, tab.session.generation, timestampLines,
    )
    setSessionLogs((current) => upsertSessionLog(current, status))
    setLoggingSessionId(undefined)
  }, [lease, loggingSessionId, tabs])

  const toggleSessionLogging = useCallback(async () => {
    if (!lease || !activeTab?.session || activeTab.state !== 'running') return
    if (!activeLog) {
      setLoggingSessionId(activeTab.session.id)
      return
    }
    try {
      const status = await backend.stopSessionLogging(
        lease.id, activeTab.session.id, activeTab.session.generation,
      )
      setSessionLogs((current) => upsertSessionLog(current, status))
    } catch (cause) {
      reportError(cause)
    }
  }, [activeLog, activeTab, lease, reportError])

  const selectTab = useCallback((tabId: string) => {
    setActiveId(tabId)
    setTabs((current) => current.map((tab) => (tab.id === tabId ? { ...tab, attention: false } : tab)))
  }, [])

  const closeTab = useCallback(
    async (tabId: string, confirmed = false) => {
      const tab = tabs.find((item) => item.id === tabId)
      if (!tab) {
        return
      }
      if (!tab.session) {
        removeTab(tabId)
        return
      }
      if (!confirmed && isLive(tab.state)) {
        setConfirmation({ kind: 'close-tab', tabId })
        return
      }
      if (!lease) return
      try {
        await backend.closeTerminal(lease.id, tab.session.id, tab.session.generation)
        removeTab(tabId)
      } catch (cause) {
        reportError(cause)
      }
    },
    [lease, removeTab, reportError, tabs],
  )

  const applyWorkspaceLayout = useCallback(async (layout: WorkspaceLayout) => {
    const sessionTabs = tabs.filter((tab): tab is TabModel & { session: Session } => Boolean(tab.session))
    if (sessionTabs.length > 0) {
      if (!lease) throw new Error('Backend is not attached')
      const closed = await Promise.allSettled(sessionTabs.map((tab) =>
        backend.closeTerminal(lease.id, tab.session.id, tab.session.generation),
      ))
      const failure = closed.find((result): result is PromiseRejectedResult => result.status === 'rejected')
      if (failure) throw new Error(`Could not close the current workspace: ${errorMessage(failure.reason)}`)
    }
    for (const controller of controllers.current.values()) controller.dispose()
    controllers.current.clear()
    setSessionLogs([])
    setLoggingSessionId(undefined)
    setSearchOpen(false)
    setSearchQuery('')
    const restored = createDisconnectedTabs(layout)
    setTabs(restored)
    setActiveId(restored[Math.min(layout.activeTab, restored.length - 1)]?.id)
    setWorkspaceMode('terminals')
  }, [lease, tabs])

  const restoreWorkspaceLayout = useCallback(async (layout: WorkspaceLayout) => {
    if (tabs.some((tab) => isLive(tab.state))) {
      setConfirmation({ kind: 'restore-layout', layoutId: layout.id })
      return
    }
    await applyWorkspaceLayout(layout)
  }, [applyWorkspaceLayout, tabs])

  const confirmClose = useCallback(async () => {
    const pending = confirmation
    setConfirmation(undefined)
    if (!pending) {
      return
    }
    if (pending.kind === 'delete-profile') {
      try {
        await backend.deleteProfile(pending.profileId)
        setProfiles((current) => current.filter((item) => item.id !== pending.profileId))
        setProfileEditor(undefined)
      } catch (cause) {
        reportError(cause)
      }
      return
    }
    if (pending.kind === 'restore-layout') {
      const layout = workspaceLayouts.find((item) => item.id === pending.layoutId)
      if (!layout) return
      try {
        await applyWorkspaceLayout(layout)
      } catch (cause) {
        reportError(cause)
      }
      return
    }
    if (!lease) {
      return
    }
    if (pending.kind === 'close-tab') {
      await closeTab(pending.tabId, true)
    } else {
      try {
        await backend.confirmApplicationClose(lease.id)
      } catch (cause) {
        reportError(cause)
      }
    }
  }, [applyWorkspaceLayout, closeTab, confirmation, lease, reportError, workspaceLayouts])

  const activeController = activeTab?.controller
  const activeHasSelection = Boolean(activeController && activeTab?.hasSelection)
  const blockingOverlayOpen = Boolean(
    sshPrompt || profileEditor || profileExchange || quickConnectOpen || loggingSessionId || confirmation,
  )
  const canOpenLocalTerminal = Boolean(
    lease && settings && localProfiles.length > 0 && !openingProfile,
  )

  const openLocalTerminal = useCallback(() => {
    const selected = localProfiles[0]
    if (canOpenLocalTerminal && selected) void connectProfile(selected)
  }, [canOpenLocalTerminal, connectProfile, localProfiles])

  const copyVisibleTerminal = useCallback(async () => {
    if (!activeController) return
    try {
      await copyVisibleText(activeController, backend.copyText)
      reportNotice('Visible terminal copied')
    } catch (cause) {
      reportError(cause)
    }
  }, [activeController, reportError, reportNotice])

  const exportTerminalSelection = useCallback(async () => {
    if (!activeController || !activeHasSelection) return
    try {
      const result = await exportSelectedText(
        activeController,
        activeTab?.title ?? 'terminal',
        backend.exportTerminalText,
      )
      if (!result.cancelled) reportNotice(`Exported ${result.filename}`)
    } catch (cause) {
      reportError(cause)
    }
  }, [activeController, activeHasSelection, activeTab, reportError, reportNotice])

  const paletteCommands = useMemo<PaletteCommand[]>(() => [
    {
      id: 'quick-connect', label: 'Quick connect', group: 'Connections', icon: Zap,
      keywords: ['ssh', 'host'], disabled: !lease || !settings || Boolean(openingProfile),
      run: () => setQuickConnectOpen(true),
    },
    {
      id: 'local-terminal', label: 'New local terminal', group: 'Connections', icon: TerminalSquare,
      keywords: ['shell', 'tab'], disabled: !canOpenLocalTerminal,
      run: openLocalTerminal,
    },
    {
      id: 'new-profile', label: 'New profile', group: 'Profiles', icon: Plus,
      keywords: ['connection', 'ssh'], run: () => setProfileEditor({}),
    },
    {
      id: 'import-profiles', label: 'Import profiles', group: 'Profiles', icon: FileUp,
      keywords: ['openssh', 'json'], disabled: Boolean(profileExchangeAction),
      run: () => void importProfiles(),
    },
    {
      id: 'export-profiles', label: 'Export profiles', group: 'Profiles', icon: FileDown,
      keywords: ['backup', 'json'], disabled: Boolean(profileExchangeAction),
      run: () => void exportProfiles(),
    },
    {
      id: 'show-terminals', label: 'Go to terminals', group: 'Navigation', icon: TerminalSquare,
      keywords: ['sessions', 'shells'], run: () => setWorkspaceMode('terminals'),
    },
    {
      id: 'show-files', label: 'Go to files', group: 'Navigation', icon: FolderOpen,
      keywords: ['sftp', 'transfers'], run: () => setWorkspaceMode('files'),
    },
    {
      id: 'show-tunnels', label: 'Go to tunnels', group: 'Navigation', icon: Network,
      keywords: ['forwarding', 'socks'], run: () => setWorkspaceMode('tunnels'),
    },
    {
      id: 'show-snippets', label: 'Go to snippets', group: 'Navigation', icon: Braces,
      keywords: ['commands'], run: () => setWorkspaceMode('snippets'),
    },
    {
      id: 'show-layouts', label: 'Go to workspace layouts', group: 'Navigation', icon: LayoutPanelTop,
      keywords: ['saved', 'restore', 'tabs'], run: () => setWorkspaceMode('layouts'),
    },
    {
      id: 'show-settings', label: 'Go to settings', group: 'Navigation', icon: Settings2,
      keywords: ['preferences', 'terminal'], run: () => setWorkspaceMode('settings'),
    },
    {
      id: 'search-terminal', label: 'Search terminal output', group: 'Active terminal', icon: Search,
      disabled: !activeController,
      run: () => {
        setWorkspaceMode('terminals')
        setSearchOpen(true)
      },
    },
    {
      id: 'copy-visible-terminal', label: 'Copy visible terminal', group: 'Active terminal', icon: Copy,
      keywords: ['clipboard', 'viewport'], disabled: !activeController,
      run: () => void copyVisibleTerminal(),
    },
    {
      id: 'export-terminal-selection', label: 'Export terminal selection', group: 'Active terminal', icon: FileDown,
      keywords: ['save', 'text'], disabled: !activeHasSelection,
      run: () => void exportTerminalSelection(),
    },
    {
      id: 'toggle-logging', label: activeLog ? 'Stop session logging' : 'Start session logging',
      group: 'Active terminal', icon: FileText, keywords: ['record', 'capture'],
      disabled: !activeTab || activeTab.state !== 'running', run: () => void toggleSessionLogging(),
    },
    {
      id: 'close-terminal', label: 'Close active terminal', group: 'Active terminal', icon: X,
      keywords: ['tab', 'session'], disabled: !activeId,
      run: () => {
        if (activeId) void closeTab(activeId)
      },
    },
    ...workspaceLayouts.map((layout) => ({
      id: `restore-layout-${layout.id}`,
      label: `Restore ${layout.name}`,
      group: 'Workspace layouts',
      icon: FolderOpen,
      keywords: ['tabs', 'disconnected'],
      run: () => void restoreWorkspaceLayout(layout).catch(reportError),
    })),
  ], [
    activeController, activeHasSelection, activeId, activeLog, activeTab, canOpenLocalTerminal,
    closeTab, copyVisibleTerminal, exportProfiles, exportTerminalSelection, importProfiles, lease,
    openLocalTerminal, openingProfile, profileExchangeAction, settings, reportError,
    restoreWorkspaceLayout, toggleSessionLogging, workspaceLayouts,
  ])

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      const modifier = isMacOS ? event.metaKey : event.ctrlKey
      if (!modifier || !event.shiftKey || event.altKey || event.repeat) return
      const key = event.key.toLocaleLowerCase()
      if (key === 'p') {
        event.preventDefault()
        if (!blockingOverlayOpen) setCommandPaletteOpen(true)
        return
      }
      if (blockingOverlayOpen || commandPaletteOpen) return
      if (key === 't' && canOpenLocalTerminal) {
        event.preventDefault()
        openLocalTerminal()
      } else if (key === 'f' && activeController) {
        event.preventDefault()
        setWorkspaceMode('terminals')
        setSearchOpen(true)
      }
    }
    document.addEventListener('keydown', handleShortcut)
    return () => document.removeEventListener('keydown', handleShortcut)
  }, [activeController, blockingOverlayOpen, canOpenLocalTerminal, commandPaletteOpen, openLocalTerminal])

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand-row">
          <div className="brand-mark" aria-hidden="true">H_</div>
          <div>
            <div className="brand-name">shh-h</div>
            <div className="brand-meta">LOCAL WORKSPACE</div>
          </div>
          <div className="brand-actions">
            <button className="icon-button" type="button" title={`Command palette (${shortcutPrefix} P)`} aria-label="Open command palette" disabled={blockingOverlayOpen} onClick={() => setCommandPaletteOpen(true)}>
              <Command size={16} />
            </button>
            <button className="icon-button" type="button" title="Quick connect" aria-label="Quick connect" disabled={!lease || !settings || Boolean(openingProfile)} onClick={() => setQuickConnectOpen(true)}>
              <Zap size={16} />
            </button>
            <button className="icon-button" type="button" title="New profile" aria-label="New profile" onClick={() => setProfileEditor({})}>
              <Plus size={17} />
            </button>
          </div>
        </div>

        <div className="sidebar-heading">
          <span className="sidebar-heading-title">Profiles <span className="count-label">{profiles.length}</span></span>
          <span className="profile-exchange-buttons">
            <button className="icon-button compact" type="button" title="Import profiles" aria-label="Import profiles" disabled={Boolean(profileExchangeAction)} onClick={() => void importProfiles()}>
              {profileExchangeAction === 'import' ? <LoaderCircle className="spin" size={14} /> : <FileUp size={14} />}
            </button>
            <button className="icon-button compact" type="button" title="Export profiles" aria-label="Export profiles" disabled={Boolean(profileExchangeAction)} onClick={() => void exportProfiles()}>
              {profileExchangeAction === 'export' ? <LoaderCircle className="spin" size={14} /> : <FileDown size={14} />}
            </button>
          </span>
        </div>
        <label className="profile-filter">
          <Search size={14} />
          <input aria-label="Filter profiles" placeholder="Filter profiles" value={profileFilter} onChange={(event) => setProfileFilter(event.target.value)} />
          {profileFilter && (
            <button type="button" aria-label="Clear profile filter" title="Clear" onClick={() => setProfileFilter('')}>
              <X size={13} />
            </button>
          )}
        </label>
        <nav className="profile-list" aria-label="Session profiles">
          {visibleProfiles.map((item) => (
            <div className="profile-item" key={item.id}>
              <button
                className="profile-row"
                type="button"
                disabled={!item.connectable || Boolean(openingProfile)}
                onClick={() => void connectProfile(item)}
              >
                <span className="profile-icon" aria-hidden="true">
                  {item.protocol === 'local' ? <Laptop size={16} /> : <TerminalSquare size={16} />}
                </span>
                <span className="profile-copy">
                  <span className="profile-name">{item.name}</span>
                  <span className="profile-endpoint">{item.endpoint}</span>
                </span>
                {openingProfile === item.id ? (
                  <LoaderCircle className="spin" size={15} />
                ) : item.favorite ? (
                  <Star className="favorite-star" size={13} fill="currentColor" />
                ) : (
                  <Command size={14} />
                )}
              </button>
              {item.protocol === 'ssh' ? (
                <button className="profile-edit" type="button" title={`Browse files on ${item.name}`} aria-label={`Browse files on ${item.name}`} disabled={Boolean(openingProfile)} onClick={() => void browseProfile(item)}>
                  <FolderOpen size={14} />
                </button>
              ) : <span className="profile-action-placeholder" />}
              <button className="profile-edit" type="button" title={`Edit ${item.name}`} aria-label={`Edit ${item.name}`} onClick={() => setProfileEditor({ profile: item })}>
                <Pencil size={14} />
              </button>
            </div>
          ))}
          {visibleProfiles.length === 0 && <div className="profile-list-empty">No matching profiles</div>}
        </nav>
        <nav className="workspace-navigation" aria-label="Workspace views">
          <button className={workspaceMode === 'terminals' ? 'is-active' : ''} type="button" onClick={() => setWorkspaceMode('terminals')}>
            <TerminalSquare size={16} /> Terminals
            {runningCount > 0 && <span>{runningCount}</span>}
          </button>
          <button className={workspaceMode === 'files' ? 'is-active' : ''} type="button" onClick={() => setWorkspaceMode('files')}>
            <FolderOpen size={16} /> Files
            {activeTransferCount > 0 && <span>{activeTransferCount}</span>}
          </button>
          <button className={workspaceMode === 'tunnels' ? 'is-active' : ''} type="button" onClick={() => setWorkspaceMode('tunnels')}>
            <Network size={16} /> Tunnels
            {activeTunnelCount > 0 && <span>{activeTunnelCount}</span>}
          </button>
          <button className={workspaceMode === 'snippets' ? 'is-active' : ''} type="button" onClick={() => setWorkspaceMode('snippets')}>
            <Braces size={16} /> Snippets
            {snippets.length > 0 && <span>{snippets.length}</span>}
          </button>
          <button className={workspaceMode === 'layouts' ? 'is-active' : ''} type="button" onClick={() => setWorkspaceMode('layouts')}>
            <LayoutPanelTop size={16} /> Layouts
            {workspaceLayouts.length > 0 && <span>{workspaceLayouts.length}</span>}
          </button>
          <button className={workspaceMode === 'settings' ? 'is-active' : ''} type="button" onClick={() => setWorkspaceMode('settings')}>
            <Settings2 size={16} /> Settings
          </button>
        </nav>
      </aside>

      <main className="workspace">
        <div className={`terminal-workspace${workspaceMode === 'terminals' ? '' : ' is-hidden'}`}>
        <div className="tabbar">
          <div className="tabs" role="tablist" aria-label="Terminal sessions">
            {tabs.map((tab) => (
              <div
                className={`tab${tab.id === activeId ? ' is-active' : ''}`}
                role="tab"
                aria-selected={tab.id === activeId}
                key={tab.id}
              >
                <button className="tab-select" type="button" onClick={() => selectTab(tab.id)}>
                  <span className={`state-dot state-${tab.state}${tab.attention ? ' has-attention' : ''}`} />
                  <span className="tab-title">{tab.title}</span>
                </button>
                <button
                  className="tab-close"
                  type="button"
                  title="Close terminal"
                  aria-label={`Close ${tab.title}`}
                  onClick={() => void closeTab(tab.id)}
                >
                  <X size={14} />
                </button>
              </div>
            ))}
          </div>
          <div className="workspace-tools">
            <button
              className={`icon-button${searchOpen ? ' is-pressed' : ''}`}
              type="button"
              title="Search terminal"
              aria-label="Search terminal"
              disabled={!activeController}
              onClick={() => setSearchOpen((value) => !value)}
            >
              <Search size={16} />
            </button>
            <button
              className="icon-button"
              type="button"
              title="Copy visible terminal"
              aria-label="Copy visible terminal"
              disabled={!activeController}
              onClick={() => void copyVisibleTerminal()}
            >
              <Copy size={16} />
            </button>
            <button
              className="icon-button"
              type="button"
              title="Export terminal selection"
              aria-label="Export terminal selection"
              disabled={!activeHasSelection}
              onClick={() => void exportTerminalSelection()}
            >
              <FileDown size={16} />
            </button>
            <button
              className={`icon-button${activeLog ? ' is-recording' : ''}`}
              type="button"
              title={activeLog ? 'Stop session logging' : 'Start session logging'}
              aria-label={activeLog ? 'Stop session logging' : 'Start session logging'}
              disabled={!activeTab || activeTab.state !== 'running'}
              onClick={() => void toggleSessionLogging()}
            >
              <FileText size={16} />
            </button>
            <button
              className="icon-button"
              type="button"
              title="New local terminal"
              aria-label="New local terminal"
              disabled={!canOpenLocalTerminal}
              onClick={openLocalTerminal}
            >
              <Plus size={17} />
            </button>
          </div>
        </div>

        {searchOpen && activeController && (
          <div className="searchbar">
            <Search size={15} />
            <input
              autoFocus
              value={searchQuery}
              aria-label="Search terminal output"
              placeholder="Find"
              onChange={(event) => {
                setSearchQuery(event.target.value)
                activeController.findNext(event.target.value)
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  if (event.shiftKey) {
                    activeController.findPrevious(searchQuery)
                  } else {
                    activeController.findNext(searchQuery)
                  }
                }
                if (event.key === 'Escape') {
                  setSearchOpen(false)
                  activeController.focus()
                }
              }}
            />
            <button className="icon-button compact" type="button" title="Previous match" aria-label="Previous match" onClick={() => activeController.findPrevious(searchQuery)}>
              <ChevronUp size={15} />
            </button>
            <button className="icon-button compact" type="button" title="Next match" aria-label="Next match" onClick={() => activeController.findNext(searchQuery)}>
              <ChevronDown size={15} />
            </button>
            <button className="icon-button compact" type="button" title="Close search" aria-label="Close search" onClick={() => setSearchOpen(false)}>
              <X size={15} />
            </button>
          </div>
        )}

        <section className="terminal-stage">
          {tabs.length === 0 ? (
            <div className="empty-state">
              <TerminalSquare size={34} strokeWidth={1.4} />
              <h1>No terminal open</h1>
              <button
                className="primary-button"
                type="button"
                disabled={!canOpenLocalTerminal}
                onClick={openLocalTerminal}
              >
                {openingProfile ? <LoaderCircle className="spin" size={16} /> : <Plus size={16} />}
                Open local terminal
              </button>
            </div>
          ) : (
            tabs.map((tab) => (
              tab.controller ? (
                <TerminalPane key={tab.id} controller={tab.controller} active={tab.id === activeId} />
              ) : tab.id === activeId ? (
                <div className="disconnected-terminal" key={tab.id}>
                  <LayoutPanelTop size={32} strokeWidth={1.4} />
                  <h1>{tab.title}</h1>
                  <span>{activeProfile?.endpoint || tab.endpoint || 'Profile unavailable'}</span>
                  <button className="primary-button" type="button" disabled={!activeProfile || Boolean(openingProfile)} onClick={() => connectRestoredTab(tab)}>
                    {openingProfile === activeProfile?.id ? <LoaderCircle className="spin" size={16} /> : <Zap size={16} />}
                    {activeProfile ? 'Connect' : 'Profile unavailable'}
                  </button>
                </div>
              ) : null
            ))
          )}
        </section>

        <footer className="statusbar">
          <span className="status-left">
            <span className={`connection-indicator${lease ? ' is-ready' : ''}`} />
            {lease ? 'Backend ready' : 'Attaching backend'}
          </span>
          <span>{runningCount} running</span>
          {activeTab?.exitSummary && <span>{activeTab.exitSummary}</span>}
          {activeLog && <span className="logging-status" title={activeLog.path}><i /> Logging</span>}
          <span className="status-spacer" />
          <span>UTF-8</span>
          <span>xterm-256color</span>
        </footer>
        </div>

        {workspaceMode === 'files' && fileSession && fileProfile ? (
          <FileBrowser
            profile={fileProfile}
            session={fileSession}
            path={remotePath}
            files={remoteFiles}
            transfers={transfers}
            favorites={fileFavorites}
            loading={fileLoading}
            onNavigate={navigateRemote}
            onRefresh={refreshRemote}
            onUpload={startUpload}
            onDownload={startDownload}
            onCreateDirectory={createRemoteDirectory}
            onRename={renameRemotePath}
            onDelete={deleteRemotePath}
            onChmod={chmodRemotePath}
            onCancelTransfer={cancelTransfer}
            onCreateFavorite={createRemotePathFavorite}
            onDeleteFavorite={deleteRemotePathFavorite}
            onClose={closeFileWorkspace}
          />
        ) : workspaceMode === 'files' ? (
          <section className="files-empty">
            <FolderOpen size={34} strokeWidth={1.4} />
            <h1>No remote files open</h1>
          </section>
        ) : null}

        {workspaceMode === 'tunnels' && (
          <TunnelWorkspace
            configs={tunnelConfigs}
            profiles={profiles.filter((profile) => profile.protocol === 'ssh')}
            snapshots={tunnelSnapshots}
            connecting={Boolean(openingProfile)}
            onCreate={createTunnel}
            onUpdate={updateTunnel}
            onDelete={deleteTunnel}
            onStart={startTunnel}
            onStop={stopTunnel}
            onRestart={restartTunnel}
          />
        )}

        {workspaceMode === 'snippets' && (
          <SnippetWorkspace
            snippets={snippets}
            targets={snippetTargets}
            onCreate={createSnippet}
            onUpdate={updateSnippet}
            onDelete={deleteSnippet}
            onRender={renderSnippet}
            onExecute={executeSnippet}
          />
        )}

        {workspaceMode === 'layouts' && (
          <WorkspaceLayoutWorkspace
            layouts={workspaceLayouts}
            savableTabCount={workspaceSnapshot.tabs.length}
            onCreate={createWorkspaceLayout}
            onRename={renameWorkspaceLayout}
            onReplace={replaceWorkspaceLayout}
            onRestore={restoreWorkspaceLayout}
            onDelete={deleteWorkspaceLayout}
          />
        )}

        {workspaceMode === 'settings' && settings && (
          <SettingsWorkspace settings={settings} onSave={saveSettings} onReset={resetSettings} />
        )}
      </main>

      {error && (
        <div className="error-toast" role="alert">
          <CircleAlert size={17} />
          <span>{error}</span>
          <button className="icon-button compact" type="button" aria-label="Dismiss error" onClick={() => setError(undefined)}>
            <X size={15} />
          </button>
        </div>
      )}

      {notice && (
        <div className="notice-toast" role="status">
          <span>{notice}</span>
          <button className="icon-button compact" type="button" aria-label="Dismiss notification" onClick={() => setNotice(undefined)}>
            <X size={15} />
          </button>
        </div>
      )}

      {commandPaletteOpen && (
        <CommandPalette commands={paletteCommands} onClose={() => setCommandPaletteOpen(false)} />
      )}

      {sshPrompt?.kind === 'trust' && (
        <SSHTrustDialog
          profile={sshPrompt.profile}
          hostKey={sshPrompt.hostKey}
          onCancel={cancelSSHConnection}
          onTrust={trustSSHHost}
        />
      )}

      {sshPrompt?.kind === 'credentials' && (
        <SSHCredentialsDialog
          profile={sshPrompt.profile}
          authentication={sshPrompt.authentication}
          onCancel={cancelSSHConnection}
          onConnect={connectWithSecret}
        />
      )}

      {profileEditor && (
        <ProfileEditor
          profile={profileEditor.profile}
          onCancel={() => setProfileEditor(undefined)}
          onSave={saveProfile}
          onDuplicate={profileEditor.profile ? () => duplicateProfile(profileEditor.profile!.id) : undefined}
          onDelete={profileEditor.profile ? () => setConfirmation({ kind: 'delete-profile', profileId: profileEditor.profile!.id }) : undefined}
        />
      )}

      {profileExchange && <ProfileExchangeDialog exchange={profileExchange} onClose={() => setProfileExchange(undefined)} />}

      {quickConnectOpen && <QuickConnectDialog onCancel={() => setQuickConnectOpen(false)} onConnect={startQuickConnect} />}

      {loggingSessionId && (
        <LoggingDialog
          title={tabs.find((tab) => tab.session?.id === loggingSessionId)?.title ?? 'Terminal'}
          onCancel={() => setLoggingSessionId(undefined)}
          onStart={startSessionLogging}
        />
      )}

      {confirmation && (
        <div className="modal-backdrop" role="presentation">
          <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
            <div className="dialog-icon"><CircleAlert size={20} /></div>
            <div className="dialog-copy">
              <h2 id="confirm-title">
                {confirmation.kind === 'close-application'
                  ? 'Close running sessions?'
                  : confirmation.kind === 'delete-profile'
                    ? 'Delete this profile?'
                    : confirmation.kind === 'restore-layout'
                      ? 'Replace the current workspace?'
                      : 'Close this terminal?'}
              </h2>
              <p>
                {confirmation.kind === 'close-application'
                  ? `${activityCount} active resource${activityCount === 1 ? '' : 's'} will be closed.`
                  : confirmation.kind === 'delete-profile'
                    ? 'The saved connection settings will be removed. Existing sessions remain open.'
                    : confirmation.kind === 'restore-layout'
                      ? `${runningCount} running terminal${runningCount === 1 ? '' : 's'} will be closed before the layout is restored.`
                      : 'The shell process and its child processes will be terminated.'}
              </p>
            </div>
            <div className="dialog-actions">
              <button className="secondary-button" type="button" autoFocus onClick={() => setConfirmation(undefined)}>Cancel</button>
              <button className="danger-button" type="button" onClick={() => void confirmClose()}>{confirmation.kind === 'restore-layout' ? 'Restore' : confirmation.kind === 'delete-profile' ? 'Delete' : 'Close'}</button>
            </div>
          </section>
        </div>
      )}
    </div>
  )
}

function updateTabState(tabs: TabModel[], event: SessionStateEvent): TabModel[] {
  return tabs.map((tab) => {
    if (tab.session?.id !== event.sessionId || tab.session.generation !== event.generation) {
      return tab
    }
    let exitSummary: string | undefined
    if (event.state === 'exited') {
      exitSummary = event.signal ? `Exited: ${event.signal}` : `Exit ${event.exitCode ?? 0}`
    } else if (event.state === 'failed') {
      exitSummary = event.message || 'Session failed'
    }
    return { ...tab, state: event.state, exitSummary }
  })
}

function isLive(state: TerminalTabState): boolean {
  return state === 'starting' || state === 'running' || state === 'closing'
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}

function filterAndSortProfiles(profiles: Profile[], filter: string): Profile[] {
  const query = filter.trim().toLocaleLowerCase()
  return profiles
    .filter((item) => {
      if (!query) {
        return true
      }
      return [item.name, item.endpoint, item.group, ...item.tags].some((value) =>
        value.toLocaleLowerCase().includes(query),
      )
    })
    .sort((left, right) => {
      if (left.favorite !== right.favorite) {
        return left.favorite ? -1 : 1
      }
      const group = left.group.localeCompare(right.group)
      return group || left.name.localeCompare(right.name)
    })
}

function upsertTransfer(transfers: Transfer[], incoming: Transfer): Transfer[] {
  const exists = transfers.some((transfer) => transfer.id === incoming.id)
  return exists
    ? transfers.map((transfer) => (transfer.id === incoming.id ? incoming : transfer))
    : [incoming, ...transfers]
}

function isLiveTunnel(state: TunnelSnapshot['state']): boolean {
  return state === 'starting' || state === 'active' || state === 'retrying'
}

function upsertTunnelSnapshot(snapshots: TunnelSnapshot[], incoming: TunnelSnapshot): TunnelSnapshot[] {
  const exists = snapshots.some((snapshot) => snapshot.configId === incoming.configId)
  return exists
    ? snapshots.map((snapshot) => snapshot.configId === incoming.configId ? incoming : snapshot)
    : [...snapshots, incoming]
}

function upsertSessionLog(statuses: SessionLogStatus[], incoming: SessionLogStatus): SessionLogStatus[] {
  const exists = statuses.some((status) =>
    status.sessionId === incoming.sessionId && status.generation === incoming.generation,
  )
  return exists
    ? statuses.map((status) => status.sessionId === incoming.sessionId && status.generation === incoming.generation
      ? incoming
      : status)
    : [...statuses, incoming]
}

function sortSnippets(snippets: Snippet[]): Snippet[] {
  return [...snippets].sort((left, right) => {
    const folder = left.folder.localeCompare(right.folder, undefined, { sensitivity: 'base' })
    return folder || left.name.localeCompare(right.name, undefined, { sensitivity: 'base' })
  })
}

function captureWorkspace(tabs: TabModel[], profiles: Profile[], activeId?: string): Pick<WorkspaceLayoutInput, 'tabs' | 'activeTab'> {
  const captured = tabs.flatMap((tab) => {
    const profile = profiles.find((item) => item.id === tab.profileId)
    return profile ? [{
      sourceId: tab.id,
      tab: { profileId: profile.id, title: tab.title || profile.name, endpoint: profile.endpoint },
    }] : []
  })
  const activeTab = Math.max(0, captured.findIndex((item) => item.sourceId === activeId))
  return { tabs: captured.map((item) => item.tab), activeTab }
}

function layoutInput(
  layout: WorkspaceLayout,
  changes: Partial<Pick<WorkspaceLayoutInput, 'name' | 'tabs' | 'activeTab'>>,
): WorkspaceLayoutInput {
  return {
    id: layout.id,
    name: changes.name ?? layout.name,
    tabs: changes.tabs ?? layout.tabs,
    activeTab: changes.activeTab ?? layout.activeTab,
  }
}

function sortWorkspaceLayouts(layouts: WorkspaceLayout[]): WorkspaceLayout[] {
  return [...layouts].sort((left, right) => left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }))
}
