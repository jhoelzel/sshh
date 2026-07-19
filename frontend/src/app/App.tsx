import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Activity,
  ArrowLeft,
  ArrowRight,
  Braces,
  ChevronDown,
  ChevronUp,
  CircleAlert,
  Columns2,
  Command,
  Copy,
  CopyPlus,
  Eraser,
  Equal,
  FileText,
  FileDown,
  FileUp,
  ExternalLink,
  FolderOpen,
  LayoutPanelTop,
  Laptop,
  ListFilter,
  LoaderCircle,
  MoreHorizontal,
  Network,
  PanelRightClose,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Rows2,
  Search,
  Settings2,
  Star,
  TerminalSquare,
  Zap,
  X,
} from 'lucide-react'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { ActivityWorkspace } from '../feature/activity/ActivityWorkspace'
import type { ActivitySession } from '../feature/activity/activityModel'
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
import { sanitizeTerminalLink } from '../feature/terminal/terminalLinks'
import { terminalTabActionAvailability } from '../feature/terminal/terminalTabActions'
import { TerminalPane } from '../feature/terminal/TerminalPane'
import { terminalPaneBodyStyle } from '../feature/terminal/terminalSplitStyles'
import { TerminalSplitOverlay } from '../feature/terminal/TerminalSplitOverlay'
import { terminalPanelId, terminalTabId } from '../feature/terminal/terminalTabIds'
import { TerminalTabs } from '../feature/terminal/TerminalTabs'
import {
  captureTerminalWorkspaceSplit,
  closeTerminalSplit,
  createTerminalWorkspace,
  nextTerminalSplitCandidate,
  removeTerminalWorkspaceTab,
  replaceTerminalWorkspaceTab,
  resizeTerminalSplit,
  restoreTerminalWorkspace,
  selectTerminalWorkspaceTab,
  splitTerminalWorkspace,
  terminalWorkspaceActiveTab,
  terminalWorkspacePane,
  terminalWorkspaceVisibleTabs,
  type SplitAxis,
  type TerminalWorkspaceState,
} from '../feature/terminal/splitLayout'
import { TunnelWorkspace } from '../feature/tunnels/TunnelWorkspace'
import { isLiveTunnelState } from '../feature/tunnels/tunnelPresentation'
import { WorkspaceLayoutWorkspace } from '../feature/workspaces/WorkspaceLayoutWorkspace'
import { backend, onCloseRequested, onSessionLog, onSessionState, onTerminalOutput, onTransfer, onTunnel } from '../lib/bridge/client'
import { loadFrontendBootstrap, loadFrontendNotificationStatus } from './frontendBootstrap'
import {
  adjacentTabId,
  createDisconnectedTabs,
  moveTabByOffset,
  reorderTabs,
  tabCycleOffset,
  type TabDropPosition,
} from './workspaces'
import type {
  AppSettings,
  BuildInfo,
  FileSession,
  FrontendLease,
  NotificationStatus,
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
  TransferResume,
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
type PaletteScope = 'commands' | 'tabs' | 'terminal'

interface TabModel {
  id: string
  profileId: string
  endpoint: string
  quick?: QuickSSHInput
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
  | { kind: 'open-link'; url: string }
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
  const [terminalWorkspace, setTerminalWorkspace] = useState(createTerminalWorkspace)
  const [workspaceMode, setWorkspaceMode] = useState<'terminals' | 'activity' | 'files' | 'tunnels' | 'snippets' | 'layouts' | 'settings'>('terminals')
  const [activeId, setActiveId] = useState<string>()
  const [openingProfile, setOpeningProfile] = useState<string>()
  const [profileEditor, setProfileEditor] = useState<ProfileEditorState>()
  const [profileExchange, setProfileExchange] = useState<ProfileExchangeResult>()
  const [profileExchangeAction, setProfileExchangeAction] = useState<'import' | 'export'>()
  const [quickConnectOpen, setQuickConnectOpen] = useState(false)
  const [paletteScope, setPaletteScope] = useState<PaletteScope>()
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
  const [transferResumes, setTransferResumes] = useState<TransferResume[]>([])
  const [tunnelConfigs, setTunnelConfigs] = useState<TunnelConfig[]>([])
  const [tunnelSnapshots, setTunnelSnapshots] = useState<TunnelSnapshot[]>([])
  const [snippets, setSnippets] = useState<Snippet[]>([])
  const [workspaceLayouts, setWorkspaceLayouts] = useState<WorkspaceLayout[]>([])
  const [sessionLogs, setSessionLogs] = useState<SessionLogStatus[]>([])
  const [settings, setSettings] = useState<AppSettings>()
  const [buildInfo, setBuildInfo] = useState<BuildInfo>()
  const [notificationStatus, setNotificationStatus] = useState<NotificationStatus>()
  const [loggingSessionId, setLoggingSessionId] = useState<string>()
  const [error, setError] = useState<string>()
  const [notice, setNotice] = useState<string>()
  const controllers = useRef(new Map<string, TerminalController>())
  const autoStartAttempted = useRef(new Set<string>())
  const terminalWorkspaceRef = useRef(terminalWorkspace)

  const updateTerminalWorkspace = useCallback(
    (update: (current: TerminalWorkspaceState) => TerminalWorkspaceState): TerminalWorkspaceState => {
      const next = update(terminalWorkspaceRef.current)
      terminalWorkspaceRef.current = next
      setTerminalWorkspace(next)
      return next
    },
    [],
  )

  const activeTab = useMemo(() => tabs.find((tab) => tab.id === activeId), [activeId, tabs])
  const activeProfile = useMemo(
    () => profiles.find((profile) => profile.id === activeTab?.profileId),
    [activeTab?.profileId, profiles],
  )
  const activeLog = useMemo(
    () => sessionLogs.find((status) => status.sessionId === activeTab?.session?.id && status.active),
    [activeTab?.session?.id, sessionLogs],
  )
  const splitCandidate = useMemo(
    () => nextTerminalSplitCandidate(tabs.map((tab) => tab.id), terminalWorkspace, activeId),
    [activeId, tabs, terminalWorkspace],
  )
  const primaryPaneTab = useMemo(
    () => tabs.find((tab) => tab.id === terminalWorkspace.primaryTabId),
    [tabs, terminalWorkspace.primaryTabId],
  )
  const secondaryPaneTab = useMemo(
    () => tabs.find((tab) => tab.id === terminalWorkspace.secondaryTabId),
    [tabs, terminalWorkspace.secondaryTabId],
  )
  const splitActive = Boolean(primaryPaneTab && secondaryPaneTab)
  const canSplitTerminal = splitActive || Boolean(primaryPaneTab && splitCandidate)
  const visibleTerminalTabIds = useMemo(
    () => terminalWorkspaceVisibleTabs(terminalWorkspace),
    [terminalWorkspace],
  )
  const activitySessions = useMemo<ActivitySession[]>(() => tabs.map((tab) => {
    const profile = profiles.find((item) => item.id === tab.profileId)
    return {
      id: tab.id,
      title: tab.title,
      endpoint: tab.endpoint,
      state: tab.state,
      startedAt: tab.session?.startedAt ?? '',
      detail: tab.exitSummary ?? '',
      selected: tab.id === activeId,
      attention: tab.attention,
      canRetry: terminalTabActionAvailability(
        tab.state,
        Boolean(tab.controller),
        Boolean(tab.quick || profile),
        Boolean(openingProfile),
      ).retry,
    }
  }), [activeId, openingProfile, profiles, tabs])
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
    () => captureWorkspace(tabs, profiles, terminalWorkspace, activeId),
    [activeId, profiles, tabs, terminalWorkspace],
  )
  const fileFavorites = useMemo(
    () => remotePathFavorites.filter((favorite) => favorite.profileId === fileProfile?.id),
    [fileProfile?.id, remotePathFavorites],
  )
  const runningCount = tabs.filter((tab) => isLive(tab.state)).length
  const activeTransferCount = transfers.filter((transfer) => transfer.state === 'queued' || transfer.state === 'running').length
  const activeTunnelCount = tunnelSnapshots.filter((snapshot) => isLiveTunnelState(snapshot.state)).length
  const globalActivityCount = runningCount + activeTransferCount + activeTunnelCount
  const activityCount = globalActivityCount + (fileSession ? 1 : 0)

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
    void loadFrontendBootstrap(frontendNonce).then((snapshot) => {
      if (!cancelled) {
        setLease(snapshot.lease)
        setProfiles(snapshot.profiles)
        setTunnelConfigs(snapshot.tunnels)
        setSnippets(snapshot.snippets)
        setWorkspaceLayouts(snapshot.workspaceLayouts)
        setRemotePathFavorites(snapshot.remotePathFavorites)
        setSettings(snapshot.settings)
        setBuildInfo(snapshot.buildInfo)
      }
    })
      .catch((cause: unknown) => {
        if (!cancelled) reportError(cause)
      })
    return () => {
      cancelled = true
    }
  }, [reportError])

  useEffect(() => {
    let cancelled = false
    void loadFrontendNotificationStatus(frontendNonce).then((status) => {
      if (!cancelled) setNotificationStatus(status)
    })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!lease) {
      return
    }
    const disposeOutput = onTerminalOutput((event) => {
      if (event?.leaseId === lease.id) {
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
      const live = tunnelSnapshots.some((snapshot) => snapshot.configId === config.id && isLiveTunnelState(snapshot.state))
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
      const fallback = next[Math.min(index, next.length - 1)]?.id
      const nextWorkspace = updateTerminalWorkspace((workspace) =>
        removeTerminalWorkspaceTab(workspace, tabId, next.map((tab) => tab.id), fallback),
      )
      setActiveId((active) => {
        if (active && active !== tabId && next.some((tab) => tab.id === active)) {
          return active
        }
        return terminalWorkspaceActiveTab(nextWorkspace) ?? fallback
      })
      return next
    })
  }, [updateTerminalWorkspace])

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
          onLinkRequested: (url) =>
            setConfirmation((current) => current ?? { kind: 'open-link', url }),
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
          quick: quick ? { ...quick } : undefined,
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
        updateTerminalWorkspace((workspace) => replaceTabId && replaced
          ? replaceTerminalWorkspaceTab(workspace, replaceTabId, session.id)
          : selectTerminalWorkspaceTab(workspace, session.id))
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
    [lease, removeTab, reportError, settings, updateTerminalWorkspace],
  )

  const openFileWorkspace = useCallback(
    async (selected: Profile, credentials: SSHCredentials = { password: '', passphrase: '' }) => {
      if (!lease) {
        throw new Error('Backend is not attached')
      }
      const opened = await backend.openSFTP(lease.id, selected.id, credentials)
      try {
        const root = canonicalRemotePath(opened.root)
        const [entries, resumes] = await Promise.all([
          backend.listRemoteFiles(lease.id, opened.id, root),
          backend.listTransferResumes(lease.id, opened.id),
        ])
        if (fileSession) {
          await backend.closeSFTP(lease.id, fileSession.id)
        }
        setFileSession(opened)
        setFileProfile(selected)
        setRemotePath(root)
        setRemoteFiles(entries)
        setTransferResumes(resumes)
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

  const browseProfile = useCallback(
    (selected: Profile) => startProfileAction(selected, { kind: 'files' }),
    [startProfileAction],
  )

  const startQuickSSHAction = useCallback(async (
    input: QuickSSHInput,
    action: Extract<SSHAction, { kind: 'terminal' }>,
    closeQuickConnect: boolean,
  ) => {
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
        setSSHPrompt({ kind: 'trust', action, profile: selected, hostKey, quick: input })
        if (closeQuickConnect) setQuickConnectOpen(false)
        return
      }
      const connected = await inspectSSHAndConnect(selected, action, input)
      if (closeQuickConnect) setQuickConnectOpen(false)
      if (connected) setOpeningProfile(undefined)
    } catch (cause) {
      setOpeningProfile(undefined)
      throw cause
    }
  }, [inspectSSHAndConnect, lease, openingProfile])

  const connectTerminalTab = useCallback(async (tab: TabModel, replace: boolean) => {
    const action: Extract<SSHAction, { kind: 'terminal' }> = {
      kind: 'terminal', replaceTabId: replace ? tab.id : undefined,
    }
    if (tab.quick) {
      await startQuickSSHAction(tab.quick, action, false)
      return
    }
    const selected = profiles.find((profile) => profile.id === tab.profileId)
    if (!selected) throw new Error('The profile used by this terminal is no longer available')
    await startProfileAction(selected, action)
  }, [profiles, startProfileAction, startQuickSSHAction])

  const connectRestoredTab = useCallback((tab: TabModel) => {
    void connectTerminalTab(tab, true).catch(reportError)
  }, [connectTerminalTab, reportError])

  const startQuickConnect = useCallback(
    (input: QuickSSHInput) => startQuickSSHAction(input, { kind: 'terminal' }, true),
    [startQuickSSHAction],
  )

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

  const resumeTransfer = useCallback(
    async (resumeId: string) => {
      if (!lease || !fileSession) throw new Error('No remote file session is open')
      const transfer = await backend.resumeTransfer(lease.id, fileSession.id, resumeId)
      setTransferResumes((current) => current.filter((resume) => resume.id !== resumeId))
      setTransfers((current) => upsertTransfer(
        current.map((item) => item.resumeId === resumeId ? { ...item, resumeId: '' } : item),
        transfer,
      ))
    },
    [fileSession, lease],
  )

  const discardTransferResume = useCallback(
    async (resumeId: string) => {
      if (!lease || !fileSession) throw new Error('No remote file session is open')
      await backend.discardTransferResume(lease.id, fileSession.id, resumeId)
      setTransferResumes((current) => current.filter((resume) => resume.id !== resumeId))
      setTransfers((current) => current.map((item) => item.resumeId === resumeId ? { ...item, resumeId: '' } : item))
    },
    [fileSession, lease],
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
    setTransferResumes([])
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

  const requestNotificationPermission = useCallback(async () => {
    const status = await backend.requestNotificationPermission()
    setNotificationStatus(status)
    return status
  }, [])

  const sendTestNotification = useCallback(() => backend.sendTestNotification(), [])

  const executeSnippet = useCallback(async (text: string, targetIds: string[], submit: boolean) => {
    const targets = targetIds.map((id) => tabs.find((tab) => tab.session?.id === id))
    if (targets.some((tab) => !tab || tab.state !== 'running' || !tab.controller)) {
      throw new Error('One or more target terminals are no longer running')
    }
    await Promise.all(targets.map((tab) => tab!.controller!.sendText(text, submit)))
    if (targetIds.length === 1) {
      const tabId = targets[0]!.id
      updateTerminalWorkspace((workspace) => selectTerminalWorkspaceTab(workspace, tabId))
      setActiveId(tabId)
    }
    setWorkspaceMode('terminals')
  }, [tabs, updateTerminalWorkspace])

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
    setWorkspaceMode('terminals')
    updateTerminalWorkspace((workspace) => selectTerminalWorkspaceTab(workspace, tabId))
    setActiveId(tabId)
    setTabs((current) => current.map((tab) => (tab.id === tabId ? { ...tab, attention: false } : tab)))
    controllers.current.get(tabId)?.focus()
  }, [updateTerminalWorkspace])

  const splitTerminal = useCallback((axis: SplitAxis) => {
    const next = updateTerminalWorkspace((workspace) => splitTerminalWorkspace(
      workspace,
      axis,
      nextTerminalSplitCandidate(tabs.map((tab) => tab.id), workspace, activeId),
    ))
    const nextActiveId = terminalWorkspaceActiveTab(next)
    if (nextActiveId) {
      setActiveId(nextActiveId)
      setTabs((current) => current.map((tab) =>
        tab.id === nextActiveId ? { ...tab, attention: false } : tab,
      ))
      controllers.current.get(nextActiveId)?.focus()
    }
    setWorkspaceMode('terminals')
  }, [activeId, tabs, updateTerminalWorkspace])

  const closeSplit = useCallback(() => {
    const next = updateTerminalWorkspace(closeTerminalSplit)
    const nextActiveId = terminalWorkspaceActiveTab(next)
    if (nextActiveId) {
      setActiveId(nextActiveId)
      controllers.current.get(nextActiveId)?.focus()
    }
  }, [updateTerminalWorkspace])

  const balanceSplit = useCallback(() => {
    const next = updateTerminalWorkspace((workspace) => resizeTerminalSplit(workspace, 0.5))
    const nextActiveId = terminalWorkspaceActiveTab(next)
    if (nextActiveId) controllers.current.get(nextActiveId)?.focus()
  }, [updateTerminalWorkspace])

  const resizeSplit = useCallback((ratio: number) => {
    updateTerminalWorkspace((workspace) => resizeTerminalSplit(workspace, ratio))
  }, [updateTerminalWorkspace])

  const selectRelativeTab = useCallback((offset: number) => {
    const nextId = adjacentTabId(tabs, activeId, offset)
    if (nextId) selectTab(nextId)
  }, [activeId, selectTab, tabs])

  const moveActiveTab = useCallback((offset: number) => {
    if (!activeId) return
    setTabs((current) => moveTabByOffset(current, activeId, offset))
  }, [activeId])

  const reorderTerminalTabs = useCallback((sourceId: string, targetId: string, position: TabDropPosition) => {
    setTabs((current) => reorderTabs(current, sourceId, targetId, position))
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

  const releaseTerminalRuntime = useCallback(async (tab: TabModel) => {
    if (tab.session) {
      if (!lease) throw new Error('Backend is not attached')
      if (tab.state !== 'closed') {
        await backend.closeTerminal(lease.id, tab.session.id, tab.session.generation)
      }
      tab.controller?.dispose()
      controllers.current.delete(tab.id)
      setSessionLogs((current) => current.filter((status) => status.sessionId !== tab.session?.id))
      setLoggingSessionId((current) => current === tab.session?.id ? undefined : current)
    }
    setSearchOpen(false)
    setSearchQuery('')
    setTabs((current) => current.map((item) => item.id === tab.id ? disconnectTerminalTab(item) : item))
  }, [lease])

  const retryTerminalTab = useCallback(async (tab: TabModel) => {
    if (openingProfile) return
    setOpeningProfile(`retry:${tab.id}`)
    setError(undefined)
    try {
      await releaseTerminalRuntime(tab)
      setOpeningProfile(undefined)
      await connectTerminalTab(tab, true)
    } catch (cause) {
      setOpeningProfile(undefined)
      reportError(cause)
    }
  }, [connectTerminalTab, openingProfile, releaseTerminalRuntime, reportError])

  const retryActivitySession = useCallback((tabId: string) => {
    const tab = tabs.find((item) => item.id === tabId)
    if (tab) void retryTerminalTab(tab)
  }, [retryTerminalTab, tabs])

  const reconnectTerminalTab = useCallback((tab: TabModel) => {
    void connectTerminalTab(tab, false).catch(reportError)
  }, [connectTerminalTab, reportError])

  const duplicateTerminalTab = useCallback((tab: TabModel) => {
    void connectTerminalTab(tab, false).catch(reportError)
  }, [connectTerminalTab, reportError])

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
    const activeTabIndex = Math.min(layout.activeTab, restored.length - 1)
    setActiveId(restored[activeTabIndex]?.id)
    updateTerminalWorkspace(() => restoreTerminalWorkspace(
      restored.map((tab) => tab.id),
      activeTabIndex,
      layout.split,
    ))
    setWorkspaceMode('terminals')
  }, [lease, tabs, updateTerminalWorkspace])

  const restoreWorkspaceLayout = useCallback(async (layout: WorkspaceLayout) => {
    if (tabs.some((tab) => isLive(tab.state))) {
      setConfirmation({ kind: 'restore-layout', layoutId: layout.id })
      return
    }
    await applyWorkspaceLayout(layout)
  }, [applyWorkspaceLayout, tabs])

  const confirmAction = useCallback(async () => {
    const pending = confirmation
    setConfirmation(undefined)
    if (!pending) {
      return
    }
    if (pending.kind === 'open-link') {
      const url = sanitizeTerminalLink(pending.url)
      if (!url) {
        reportError(new Error('The terminal link is no longer valid.'))
        return
      }
      try {
        BrowserOpenURL(url)
      } catch (cause) {
        reportError(cause)
      }
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
  const activeHasConnectionDescriptor = Boolean(activeTab && (activeTab.quick || activeProfile))
  const terminalActionAvailability = terminalTabActionAvailability(
    activeTab?.state,
    Boolean(activeController),
    activeHasConnectionDescriptor,
    Boolean(openingProfile),
  )
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

  const clearActiveScrollback = useCallback(() => {
    if (!activeController) return
    activeController.clearScrollback()
    setSearchOpen(false)
    setSearchQuery('')
    reportNotice('Terminal scrollback cleared')
  }, [activeController, reportNotice])

  const resetActiveTerminal = useCallback(() => {
    if (!activeController) return
    activeController.resetTerminal()
    setSearchOpen(false)
    setSearchQuery('')
    reportNotice('Terminal reset')
  }, [activeController, reportNotice])

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

  const activeTabIndex = tabs.findIndex((tab) => tab.id === activeId)
  const tabCommands = useMemo<PaletteCommand[]>(() => tabs.map((tab, index) => ({
    id: `switch-tab-${tab.id}`,
    label: `Switch to ${index + 1}: ${tab.title}`,
    group: 'Terminal tabs',
    icon: TerminalSquare,
    keywords: [tab.endpoint, tab.profileId, tab.state, `tab ${index + 1}`],
    run: () => selectTab(tab.id),
  })), [selectTab, tabs])

  const terminalActionCommands = useMemo<PaletteCommand[]>(() => [
    {
      id: 'split-terminal-right', label: 'Split terminal right', group: 'Layout', icon: Columns2,
      keywords: ['pane', 'side by side', 'vertical'], disabled: !canSplitTerminal,
      run: () => splitTerminal('row'),
    },
    {
      id: 'split-terminal-down', label: 'Split terminal down', group: 'Layout', icon: Rows2,
      keywords: ['pane', 'stacked', 'horizontal'], disabled: !canSplitTerminal,
      run: () => splitTerminal('column'),
    },
    {
      id: 'balance-terminal-split', label: 'Balance terminal panes', group: 'Layout', icon: Equal,
      keywords: ['equal', 'half', 'resize'], disabled: !splitActive || terminalWorkspace.ratio === 0.5,
      run: balanceSplit,
    },
    {
      id: 'close-terminal-split', label: 'Close terminal split', group: 'Layout', icon: PanelRightClose,
      keywords: ['single pane', 'unsplit'], disabled: !splitActive,
      run: closeSplit,
    },
    {
      id: 'retry-terminal', label: 'Retry terminal', group: 'Connection', icon: RefreshCw,
      keywords: ['replace', 'reconnect'], disabled: !terminalActionAvailability.retry,
      run: () => {
        if (activeTab) void retryTerminalTab(activeTab)
      },
    },
    {
      id: 'reconnect-terminal-new-tab', label: 'Reconnect in new tab', group: 'Connection', icon: Plus,
      keywords: ['retry', 'preserve history'], disabled: !terminalActionAvailability.reconnectInNewTab,
      run: () => {
        if (activeTab) reconnectTerminalTab(activeTab)
      },
    },
    {
      id: 'duplicate-terminal-tab', label: 'Duplicate terminal tab', group: 'Connection', icon: CopyPlus,
      keywords: ['clone', 'new session'], disabled: !terminalActionAvailability.duplicate,
      run: () => {
        if (activeTab) duplicateTerminalTab(activeTab)
      },
    },
    {
      id: 'clear-terminal-scrollback', label: 'Clear scrollback', group: 'Display', icon: Eraser,
      keywords: ['history', 'buffer'], disabled: !terminalActionAvailability.clearScrollback,
      run: clearActiveScrollback,
    },
    {
      id: 'reset-terminal', label: 'Reset terminal', group: 'Display', icon: RotateCcw,
      keywords: ['ris', 'emulator'], disabled: !terminalActionAvailability.reset,
      run: resetActiveTerminal,
    },
  ], [
    activeTab, balanceSplit, canSplitTerminal, clearActiveScrollback, closeSplit, duplicateTerminalTab,
    reconnectTerminalTab, resetActiveTerminal, retryTerminalTab, splitActive, splitTerminal, terminalActionAvailability,
    terminalWorkspace.ratio,
  ])

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
      id: 'show-activity', label: 'Go to activity', group: 'Navigation', icon: Activity,
      keywords: ['sessions', 'transfers', 'tunnels', 'resources'], run: () => setWorkspaceMode('activity'),
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
      id: 'previous-terminal-tab', label: 'Previous terminal tab', group: 'Navigation', icon: ArrowLeft,
      keywords: ['cycle', 'switch'], disabled: tabs.length < 2, run: () => selectRelativeTab(-1),
    },
    {
      id: 'next-terminal-tab', label: 'Next terminal tab', group: 'Navigation', icon: ArrowRight,
      keywords: ['cycle', 'switch'], disabled: tabs.length < 2, run: () => selectRelativeTab(1),
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
    ...terminalActionCommands,
    {
      id: 'close-terminal', label: 'Close active terminal', group: 'Active terminal', icon: X,
      keywords: ['tab', 'session'], disabled: !activeId,
      run: () => {
        if (activeId) void closeTab(activeId)
      },
    },
    {
      id: 'move-terminal-tab-left', label: 'Move active tab left', group: 'Active terminal', icon: ArrowLeft,
      keywords: ['reorder'], disabled: activeTabIndex <= 0, run: () => moveActiveTab(-1),
    },
    {
      id: 'move-terminal-tab-right', label: 'Move active tab right', group: 'Active terminal', icon: ArrowRight,
      keywords: ['reorder'], disabled: activeTabIndex < 0 || activeTabIndex >= tabs.length - 1,
      run: () => moveActiveTab(1),
    },
    ...tabCommands,
    ...workspaceLayouts.map((layout) => ({
      id: `restore-layout-${layout.id}`,
      label: `Restore ${layout.name}`,
      group: 'Workspace layouts',
      icon: FolderOpen,
      keywords: ['tabs', 'disconnected'],
      run: () => void restoreWorkspaceLayout(layout).catch(reportError),
    })),
  ], [
    activeController, activeHasSelection, activeId, activeLog, activeTab, activeTabIndex, canOpenLocalTerminal,
    closeTab, copyVisibleTerminal, exportProfiles, exportTerminalSelection, importProfiles, lease,
    moveActiveTab, openLocalTerminal, openingProfile, profileExchangeAction, settings, reportError,
    restoreWorkspaceLayout, selectRelativeTab, tabCommands, tabs.length, terminalActionCommands,
    toggleSessionLogging, workspaceLayouts,
  ])

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      const cycleOffset = tabCycleOffset(event)
      if (cycleOffset && !blockingOverlayOpen && !paletteScope && tabs.length > 0) {
        event.preventDefault()
        event.stopPropagation()
        if (tabs.length > 1) selectRelativeTab(cycleOffset)
        return
      }
      const modifier = isMacOS ? event.metaKey : event.ctrlKey
      if (!modifier || !event.shiftKey || event.altKey || event.repeat) return
      const key = event.key.toLocaleLowerCase()
      if (key === 'p') {
        event.preventDefault()
        if (!blockingOverlayOpen) setPaletteScope('commands')
        return
      }
      if (blockingOverlayOpen || paletteScope) return
      if (key === 't' && canOpenLocalTerminal) {
        event.preventDefault()
        openLocalTerminal()
      } else if (key === 'f' && activeController) {
        event.preventDefault()
        setWorkspaceMode('terminals')
        setSearchOpen(true)
      }
    }
    document.addEventListener('keydown', handleShortcut, true)
    return () => document.removeEventListener('keydown', handleShortcut, true)
  }, [
    activeController, blockingOverlayOpen, canOpenLocalTerminal, openLocalTerminal, paletteScope,
    selectRelativeTab, tabs.length,
  ])

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
            <button className="icon-button" type="button" title={`Command palette (${shortcutPrefix} P)`} aria-label="Open command palette" disabled={blockingOverlayOpen} onClick={() => setPaletteScope('commands')}>
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
          <button className={workspaceMode === 'activity' ? 'is-active' : ''} type="button" onClick={() => setWorkspaceMode('activity')}>
            <Activity size={16} /> Activity
            {globalActivityCount > 0 && <span>{globalActivityCount}</span>}
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
          <TerminalTabs
            tabs={tabs}
            activeId={activeId}
            visibleIds={visibleTerminalTabIds}
            onSelect={selectTab}
            onClose={(tabId) => void closeTab(tabId)}
            onReorder={reorderTerminalTabs}
          />
          <div className="workspace-tools">
            <button
              className={`icon-button${splitActive && terminalWorkspace.axis === 'row' ? ' is-pressed' : ''}`}
              type="button"
              title="Split terminal right"
              aria-label="Split terminal right"
              disabled={!canSplitTerminal}
              onClick={() => splitTerminal('row')}
            >
              <Columns2 size={16} />
            </button>
            <button
              className={`icon-button${splitActive && terminalWorkspace.axis === 'column' ? ' is-pressed' : ''}`}
              type="button"
              title="Split terminal down"
              aria-label="Split terminal down"
              disabled={!canSplitTerminal}
              onClick={() => splitTerminal('column')}
            >
              <Rows2 size={16} />
            </button>
            <button
              className="icon-button"
              type="button"
              title="Close terminal split"
              aria-label="Close terminal split"
              disabled={!splitActive}
              onClick={closeSplit}
            >
              <PanelRightClose size={16} />
            </button>
            <button
              className="icon-button"
              type="button"
              title="Find terminal tab"
              aria-label="Find terminal tab"
              disabled={tabs.length === 0}
              onClick={() => setPaletteScope('tabs')}
            >
              <ListFilter size={16} />
            </button>
            <button
              className="icon-button"
              type="button"
              title="Terminal actions"
              aria-label="Terminal actions"
              disabled={!activeTab}
              onClick={() => setPaletteScope('terminal')}
            >
              <MoreHorizontal size={17} />
            </button>
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
            tabs.map((tab) => {
              const pane = terminalWorkspacePane(terminalWorkspace, tab.id)
              const visible = Boolean(pane)
              const style = pane && splitActive ? terminalPaneBodyStyle(terminalWorkspace, pane) : undefined
              const tabProfile = profiles.find((profile) => profile.id === tab.profileId)
              return tab.controller ? (
                <TerminalPane
                  key={tab.id}
                  controller={tab.controller}
                  visible={visible}
                  selected={tab.id === activeId}
                  tabId={tab.id}
                  style={style}
                />
              ) : (
                <div
                  id={terminalPanelId(tab.id)}
                  className="disconnected-terminal"
                  role="tabpanel"
                  aria-hidden={!visible}
                  aria-labelledby={terminalTabId(tab.id)}
                  hidden={!visible}
                  key={tab.id}
                  style={style}
                >
                  <LayoutPanelTop size={32} strokeWidth={1.4} />
                  <h1>{tab.title}</h1>
                  <span>{tabProfile?.endpoint || tab.endpoint || 'Profile unavailable'}</span>
                  <button className="primary-button" type="button" disabled={!tabProfile || Boolean(openingProfile)} onClick={() => connectRestoredTab(tab)}>
                    {openingProfile === tabProfile?.id ? <LoaderCircle className="spin" size={16} /> : <Zap size={16} />}
                    {tabProfile ? 'Connect' : 'Profile unavailable'}
                  </button>
                </div>
              )
            })
          )}
          {splitActive && primaryPaneTab && secondaryPaneTab && (
            <TerminalSplitOverlay
              workspace={terminalWorkspace}
              primary={{
                tabId: primaryPaneTab.id,
                title: primaryPaneTab.title,
                state: primaryPaneTab.state,
                attention: primaryPaneTab.attention,
              }}
              secondary={{
                tabId: secondaryPaneTab.id,
                title: secondaryPaneTab.title,
                state: secondaryPaneTab.state,
                attention: secondaryPaneTab.attention,
              }}
              activeTabId={activeId}
              onActivate={selectTab}
              onRatioChange={resizeSplit}
            />
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

        {workspaceMode === 'activity' && (
          <ActivityWorkspace
            sessions={activitySessions}
            transfers={transfers}
            tunnelConfigs={tunnelConfigs}
            tunnelSnapshots={tunnelSnapshots}
            profiles={profiles}
            fileSessionId={fileSession?.id}
            connecting={Boolean(openingProfile)}
            onOpenSession={selectTab}
            onRetrySession={retryActivitySession}
            onCloseSession={(tabId) => void closeTab(tabId)}
            onCancelTransfer={cancelTransfer}
            onOpenFiles={() => setWorkspaceMode('files')}
            onOpenTunnels={() => setWorkspaceMode('tunnels')}
            onStartTunnel={startTunnel}
            onStopTunnel={stopTunnel}
            onRestartTunnel={restartTunnel}
          />
        )}

        {workspaceMode === 'files' && fileSession && fileProfile ? (
          <FileBrowser
            profile={fileProfile}
            session={fileSession}
            path={remotePath}
            files={remoteFiles}
            transfers={transfers}
            resumes={transferResumes}
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
            onResumeTransfer={resumeTransfer}
            onDiscardResume={discardTransferResume}
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

        {workspaceMode === 'settings' && settings && buildInfo && (
          <SettingsWorkspace
            settings={settings}
            buildInfo={buildInfo}
            notificationStatus={notificationStatus}
            onSave={saveSettings}
            onReset={resetSettings}
            onRequestNotificationPermission={requestNotificationPermission}
            onSendTestNotification={sendTestNotification}
          />
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

      {paletteScope && (
        <CommandPalette
          commands={paletteScope === 'tabs'
            ? tabCommands
            : paletteScope === 'terminal'
              ? terminalActionCommands
              : paletteCommands}
          emptyLabel={paletteScope === 'tabs'
            ? 'No matching terminal tabs'
            : paletteScope === 'terminal'
              ? 'No matching terminal actions'
              : undefined}
          onClose={() => setPaletteScope(undefined)}
          searchLabel={paletteScope === 'tabs'
            ? 'Search terminal tabs'
            : paletteScope === 'terminal'
              ? 'Search terminal actions'
              : undefined}
          title={paletteScope === 'tabs'
            ? 'Find terminal tab'
            : paletteScope === 'terminal'
              ? 'Terminal actions'
              : undefined}
        />
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
                  : confirmation.kind === 'open-link'
                    ? 'Open external link?'
                  : confirmation.kind === 'delete-profile'
                    ? 'Delete this profile?'
                    : confirmation.kind === 'restore-layout'
                      ? 'Replace the current workspace?'
                      : 'Close this terminal?'}
              </h2>
              <p>
                {confirmation.kind === 'close-application'
                  ? `${activityCount} active resource${activityCount === 1 ? '' : 's'} will be closed.`
                  : confirmation.kind === 'open-link'
                    ? 'This address came from terminal output and will open in your system browser.'
                  : confirmation.kind === 'delete-profile'
                    ? 'The saved connection settings will be removed. Existing sessions remain open.'
                    : confirmation.kind === 'restore-layout'
                      ? `${runningCount} running terminal${runningCount === 1 ? '' : 's'} will be closed before the layout is restored.`
                      : 'The shell process and its child processes will be terminated.'}
              </p>
              {confirmation.kind === 'open-link' && (
                <code className="terminal-link-address">{confirmation.url}</code>
              )}
            </div>
            <div className="dialog-actions">
              <button className="secondary-button" type="button" autoFocus onClick={() => setConfirmation(undefined)}>Cancel</button>
              <button
                className={confirmation.kind === 'open-link' ? 'primary-button' : 'danger-button'}
                type="button"
                onClick={() => void confirmAction()}
              >
                {confirmation.kind === 'open-link' && <ExternalLink size={14} />}
                {confirmation.kind === 'open-link' ? 'Open link' : confirmation.kind === 'restore-layout' ? 'Restore' : confirmation.kind === 'delete-profile' ? 'Delete' : 'Close'}
              </button>
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

function disconnectTerminalTab(tab: TabModel): TabModel {
  return {
    id: tab.id,
    profileId: tab.profileId,
    endpoint: tab.endpoint,
    quick: tab.quick,
    title: tab.title,
    state: 'disconnected',
    exitSummary: tab.exitSummary,
    attention: false,
    hasSelection: false,
  }
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

function captureWorkspace(
  tabs: TabModel[],
  profiles: Profile[],
  terminalWorkspace: TerminalWorkspaceState,
  activeId?: string,
): Pick<WorkspaceLayoutInput, 'tabs' | 'activeTab' | 'split'> {
  const captured = tabs.flatMap((tab) => {
    const profile = profiles.find((item) => item.id === tab.profileId)
    return profile ? [{
      sourceId: tab.id,
      tab: { profileId: profile.id, title: tab.title || profile.name, endpoint: profile.endpoint },
    }] : []
  })
  const activeTab = Math.max(0, captured.findIndex((item) => item.sourceId === activeId))
  const tabIndexes = new Map(captured.map((item, index) => [item.sourceId, index]))
  const split = captureTerminalWorkspaceSplit(terminalWorkspace, tabIndexes, activeId)
  return { tabs: captured.map((item) => item.tab), activeTab, split }
}

function layoutInput(
  layout: WorkspaceLayout,
  changes: Partial<Pick<WorkspaceLayoutInput, 'name' | 'tabs' | 'activeTab' | 'split'>>,
): WorkspaceLayoutInput {
  return {
    id: layout.id,
    name: changes.name ?? layout.name,
    tabs: changes.tabs ?? layout.tabs,
    activeTab: changes.activeTab ?? layout.activeTab,
    split: Object.hasOwn(changes, 'split') ? changes.split : layout.split,
  }
}

function sortWorkspaceLayouts(layouts: WorkspaceLayout[]): WorkspaceLayout[] {
  return [...layouts].sort((left, right) => left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }))
}
