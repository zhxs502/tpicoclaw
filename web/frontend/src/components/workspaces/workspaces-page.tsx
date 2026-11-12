import {
  IconAlertTriangle,
  IconEdit,
  IconFolder,
  IconKey,
  IconLoader2,
  IconPlus,
  IconShieldLock,
  IconTrash,
} from "@tabler/icons-react"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { restartGateway } from "@/api/gateway"
import {
  type WorkspaceProfile,
  type WorkspaceProfileInput,
  type WorkspacesResponse,
  getWorkspaces,
  updateWorkspaces,
} from "@/api/workspaces"
import { PageHeader } from "@/components/page-header"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"
import { refreshGatewayState } from "@/store/gateway"

import { ProfileDialog } from "./profile-dialog"

const DEFAULT_WORKSPACE_OPTION = "__default__"

function toInput(profile: WorkspaceProfile): WorkspaceProfileInput {
  return {
    id: profile.id,
    name: profile.name ?? profile.id,
    path: profile.path,
    restrict_to_workspace: profile.restrict_to_workspace ?? true,
    allow_read_outside_workspace: profile.allow_read_outside_workspace ?? false,
    api_key_fingerprints: [...(profile.api_key_fingerprints ?? [])],
  }
}

export function WorkspacesPage() {
  const { t } = useTranslation()
  const [data, setData] = useState<WorkspacesResponse | null>(null)
  const [profiles, setProfiles] = useState<WorkspaceProfileInput[]>([])
  const [enabled, setEnabled] = useState(true)
  const [dirty, setDirty] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [restartRequired, setRestartRequired] = useState(false)
  const [error, setError] = useState("")

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingProfile, setEditingProfile] =
    useState<WorkspaceProfileInput | null>(null)
  const [deletingProfile, setDeletingProfile] =
    useState<WorkspaceProfileInput | null>(null)
  const saveChain = useRef<Promise<void>>(Promise.resolve())
  const saveSeq = useRef(0)

  const applyResponse = useCallback((response: WorkspacesResponse) => {
    setData(response)
    setEnabled(response.enabled)
    setProfiles(response.profiles.map(toInput))
    setDirty(false)
  }, [])

  const syncGatewayState = useCallback(async () => {
    const gateway = await refreshGatewayState({ force: true })
    setRestartRequired(gateway?.restartRequired === true)
    return gateway
  }, [])

  const fetchWorkspaces = useCallback(async () => {
    setLoading(true)
    try {
      applyResponse(await getWorkspaces())
      await syncGatewayState()
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : t("workspaces.loadError"))
    } finally {
      setLoading(false)
    }
  }, [applyResponse, syncGatewayState, t])

  useEffect(() => {
    void fetchWorkspaces()
  }, [fetchWorkspaces])

  const keys = data?.keys ?? []

  const profileForFingerprint = useCallback(
    (fingerprint: string) =>
      profiles.find((profile) =>
        profile.api_key_fingerprints.includes(fingerprint),
      )?.id ?? "",
    [profiles],
  )

  const profileName = useCallback(
    (id: string) => profiles.find((profile) => profile.id === id)?.name || id,
    [profiles],
  )

  const persist = useCallback(
    (
      nextEnabled: boolean,
      nextProfiles: WorkspaceProfileInput[],
      options?: { notify?: boolean },
    ) => {
      const seq = ++saveSeq.current
      setSaving(true)
      saveChain.current = saveChain.current.then(async () => {
        try {
          const response = await updateWorkspaces({
            enabled: nextEnabled,
            profiles: nextProfiles.map((profile) => ({
              ...profile,
              restrict_to_workspace: profile.restrict_to_workspace ?? true,
              allow_read_outside_workspace:
                profile.allow_read_outside_workspace ?? false,
            })),
          })
          if (seq !== saveSeq.current) return
          applyResponse(response)
          const gateway = await syncGatewayState()
          if (options?.notify) {
            showSaveSuccessOrRestartToast(
              t,
              t("workspaces.saved"),
              t("navigation.workspaces"),
              gateway?.restartRequired === true,
            )
          }
        } catch (err) {
          if (seq !== saveSeq.current) return
          setDirty(true)
          toast.error(
            err instanceof Error ? err.message : t("workspaces.saveError"),
          )
        } finally {
          if (seq === saveSeq.current) setSaving(false)
        }
      })
      return saveChain.current
    },
    [applyResponse, syncGatewayState, t],
  )

  const handleBindingChange = (fingerprint: string, value: string) => {
    const target = value === DEFAULT_WORKSPACE_OPTION ? "" : value
    const nextProfiles = profiles.map((profile) => {
      const withoutFingerprint = profile.api_key_fingerprints.filter(
        (item) => item !== fingerprint,
      )
      if (profile.id !== target) {
        return { ...profile, api_key_fingerprints: withoutFingerprint }
      }
      return {
        ...profile,
        api_key_fingerprints: [...withoutFingerprint, fingerprint],
      }
    })
    // Assigning a key to a workspace implies that isolation is wanted.
    const nextEnabled = target ? true : enabled
    setProfiles(nextProfiles)
    setEnabled(nextEnabled)
    setDirty(true)
    void persist(nextEnabled, nextProfiles)
  }

  const handleSubmitProfile = (profile: WorkspaceProfileInput) => {
    const exists = profiles.some((item) => item.id === profile.id)
    const nextProfiles = exists
      ? profiles.map((item) =>
          item.id === profile.id
            ? { ...profile, api_key_fingerprints: item.api_key_fingerprints }
            : item,
        )
      : [...profiles, profile]
    const nextEnabled = exists ? enabled : true
    setProfiles(nextProfiles)
    setEnabled(nextEnabled)
    setDialogOpen(false)
    setEditingProfile(null)
    setDirty(true)
    void persist(nextEnabled, nextProfiles)
  }

  const handleDeleteProfile = () => {
    if (!deletingProfile) return
    const nextProfiles = profiles.filter(
      (profile) => profile.id !== deletingProfile.id,
    )
    const nextEnabled = nextProfiles.length === 0 ? false : enabled
    setProfiles(nextProfiles)
    setEnabled(nextEnabled)
    setDeletingProfile(null)
    setDirty(true)
    void persist(nextEnabled, nextProfiles)
  }

  const handleSave = () => {
    void persist(enabled, profiles, { notify: true })
  }

  const handleRestart = async () => {
    setRestarting(true)
    try {
      await restartGateway()
      toast.success(t("workspaces.restartDone"))
      await syncGatewayState()
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("workspaces.restartError"),
      )
    } finally {
      setRestarting(false)
    }
  }

  const warnings = useMemo(() => data?.warnings ?? [], [data])
  const defaultKey = keys.find((key) => key.is_default_model)
  const defaultKeyUnbound = Boolean(
    enabled && defaultKey && !profileForFingerprint(defaultKey.fingerprint),
  )

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.workspaces")}>
        <div className="flex items-center gap-2">
          {dirty && (
            <Badge variant="outline" className="text-muted-foreground">
              {t("workspaces.unsaved")}
            </Badge>
          )}
          <Button
            size="sm"
            onClick={() => void handleSave()}
            disabled={!dirty || saving || loading}
          >
            {saving && <IconLoader2 className="size-4 animate-spin" />}
            {t("common.save")}
          </Button>
        </div>
      </PageHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-10 sm:px-6">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 pt-2">
          <p className="text-muted-foreground text-sm">
            {t("workspaces.description")}
          </p>
          <p className="text-muted-foreground/80 -mt-3 text-xs">
            {t("workspaces.applyHint")}
          </p>

          {loading && (
            <div className="flex items-center justify-center py-20">
              <IconLoader2 className="text-muted-foreground size-6 animate-spin" />
            </div>
          )}

          {error && !loading && (
            <div className="bg-destructive/10 rounded-lg px-4 py-3 text-sm">
              <p className="text-destructive">{error}</p>
              <Button
                size="sm"
                variant="outline"
                className="mt-3"
                onClick={() => void fetchWorkspaces()}
              >
                {t("workspaces.retry")}
              </Button>
            </div>
          )}

          {!loading && !error && data && (
            <>
              {restartRequired && (
                <div className="flex flex-col gap-3 rounded-xl border border-amber-500/40 bg-amber-500/10 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex items-start gap-2 text-sm text-amber-700 dark:text-amber-300">
                    <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
                    <span>{t("workspaces.restartPending")}</span>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void handleRestart()}
                    disabled={restarting}
                  >
                    {restarting && (
                      <IconLoader2 className="size-4 animate-spin" />
                    )}
                    {t("workspaces.restartNow")}
                  </Button>
                </div>
              )}

              <section className="rounded-xl border p-4">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-2 text-sm font-medium">
                      <IconShieldLock className="size-4 opacity-70" />
                      {t("workspaces.enable.title")}
                    </div>
                    <p className="text-muted-foreground mt-1 text-sm">
                      {t("workspaces.enable.description")}
                    </p>
                    {!enabled && profiles.length > 0 && (
                      <p className="mt-1 text-sm text-amber-600 dark:text-amber-400">
                        {t("workspaces.disabledHint")}
                      </p>
                    )}
                  </div>
                  <Switch
                    checked={enabled}
                    onCheckedChange={(value) => {
                      setEnabled(value)
                      setDirty(true)
                      void persist(value, profiles)
                    }}
                  />
                </div>

                <div className="mt-4 grid gap-3 text-xs sm:grid-cols-2">
                  <div className="bg-muted/40 rounded-lg p-3">
                    <p className="text-muted-foreground">
                      {t("workspaces.defaultWorkspace")}
                    </p>
                    <p className="mt-1 font-mono break-all">
                      {data.default_workspace}
                    </p>
                  </div>
                  <div className="bg-muted/40 rounded-lg p-3">
                    <p className="text-muted-foreground">
                      {t("workspaces.activeWorkspace")}
                    </p>
                    <div className="mt-1 flex flex-wrap items-center gap-2">
                      <span className="font-mono break-all">
                        {data.active_workspace}
                      </span>
                      {data.active_profile_id && (
                        <Badge variant="secondary">
                          {profileName(data.active_profile_id)}
                        </Badge>
                      )}
                    </div>
                    {data.default_model && (
                      <p className="text-muted-foreground mt-1">
                        {t("workspaces.activeModel", {
                          model: data.default_model,
                        })}
                      </p>
                    )}
                    {defaultKeyUnbound && (
                      <p className="mt-1 text-amber-600 dark:text-amber-400">
                        {t("workspaces.activeUnbound", {
                          model: data.default_model,
                        })}
                      </p>
                    )}
                  </div>
                </div>

                {warnings.length > 0 && (
                  <div className="mt-3 flex flex-col gap-1">
                    {warnings.map((warning) => (
                      <p
                        key={warning}
                        className="flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400"
                      >
                        <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                        {warning}
                      </p>
                    ))}
                  </div>
                )}
              </section>

              <section>
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <h3 className="text-sm font-semibold">
                      {t("workspaces.profiles.title")}
                    </h3>
                    <p className="text-muted-foreground mt-1 text-sm">
                      {t("workspaces.profiles.description")}
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setEditingProfile(null)
                      setDialogOpen(true)
                    }}
                  >
                    <IconPlus className="size-4" />
                    {t("workspaces.profiles.add")}
                  </Button>
                </div>

                {profiles.length === 0 ? (
                  <div className="text-muted-foreground mt-3 rounded-xl border border-dashed p-6 text-center text-sm">
                    {t("workspaces.profiles.empty")}
                  </div>
                ) : (
                  <div className="mt-3 grid gap-3 sm:grid-cols-2">
                    {profiles.map((profile) => (
                      <div
                        key={profile.id}
                        className="flex flex-col gap-2 rounded-xl border p-4"
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div className="flex min-w-0 items-center gap-2">
                            <IconFolder className="text-muted-foreground size-4 shrink-0" />
                            <span className="truncate text-sm font-semibold">
                              {profile.name || profile.id}
                            </span>
                            <span className="text-muted-foreground shrink-0 font-mono text-[10px]">
                              {profile.id}
                            </span>
                          </div>
                          <div className="flex shrink-0 items-center gap-0.5">
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={t("workspaces.profiles.edit")}
                              title={t("workspaces.profiles.edit")}
                              onClick={() => {
                                setEditingProfile(profile)
                                setDialogOpen(true)
                              }}
                            >
                              <IconEdit className="size-3.5" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={t("workspaces.profiles.delete")}
                              title={t("workspaces.profiles.delete")}
                              className="text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                              onClick={() => setDeletingProfile(profile)}
                            >
                              <IconTrash className="size-3.5" />
                            </Button>
                          </div>
                        </div>

                        <p className="text-muted-foreground font-mono text-xs break-all">
                          {profile.path}
                        </p>

                        <div className="flex flex-wrap items-center gap-1.5">
                          <Badge variant="secondary">
                            <IconKey className="size-3" />
                            {t("workspaces.profiles.keyCount", {
                              count: profile.api_key_fingerprints.length,
                            })}
                          </Badge>
                          <Badge
                            variant="outline"
                            className={
                              profile.restrict_to_workspace
                                ? "text-emerald-600 dark:text-emerald-400"
                                : "text-muted-foreground"
                            }
                          >
                            {profile.restrict_to_workspace
                              ? t("workspaces.profiles.restricted")
                              : t("workspaces.profiles.unrestricted")}
                          </Badge>
                          {profile.allow_read_outside_workspace && (
                            <Badge
                              variant="outline"
                              className="text-amber-600 dark:text-amber-400"
                            >
                              {t("workspaces.profiles.readOutside")}
                            </Badge>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </section>

              <section>
                <h3 className="text-sm font-semibold">
                  {t("workspaces.keys.title")}
                </h3>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("workspaces.keys.description")}
                </p>

                {keys.length === 0 ? (
                  <div className="text-muted-foreground mt-3 rounded-xl border border-dashed p-6 text-center text-sm">
                    {t("workspaces.keys.empty")}
                  </div>
                ) : (
                  <div className="mt-3 overflow-x-auto rounded-xl border">
                    <table className="w-full text-left text-sm">
                      <thead className="bg-muted/40 text-muted-foreground text-xs">
                        <tr>
                          <th className="px-4 py-2.5 font-medium">
                            {t("workspaces.keys.model")}
                          </th>
                          <th className="px-4 py-2.5 font-medium">
                            {t("workspaces.keys.apiKey")}
                          </th>
                          <th className="px-4 py-2.5 font-medium">
                            {t("workspaces.keys.workspace")}
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {keys.map((key) => {
                          const boundProfile = profileForFingerprint(
                            key.fingerprint,
                          )
                          return (
                            <tr
                              key={key.fingerprint}
                              className="border-t align-top"
                            >
                              <td className="px-4 py-3">
                                <div className="flex items-center gap-2">
                                  <span className="font-medium">
                                    {key.model_name}
                                  </span>
                                  {key.is_default_model && (
                                    <Badge variant="secondary">
                                      {t("workspaces.keys.defaultModel")}
                                    </Badge>
                                  )}
                                </div>
                                <div className="text-muted-foreground mt-0.5 font-mono text-[11px]">
                                  {key.provider}/{key.model}
                                  {key.key_index > 0
                                    ? ` · #${key.key_index + 1}`
                                    : ""}
                                </div>
                              </td>
                              <td className="px-4 py-3">
                                <div className="flex items-center gap-1.5 font-mono text-xs">
                                  <IconKey className="text-muted-foreground size-3.5" />
                                  {key.key_mask}
                                </div>
                                <div className="text-muted-foreground mt-0.5 font-mono text-[10px]">
                                  {key.fingerprint}
                                </div>
                              </td>
                              <td className="px-4 py-3">
                                <Select
                                  value={
                                    boundProfile || DEFAULT_WORKSPACE_OPTION
                                  }
                                  onValueChange={(value) =>
                                    handleBindingChange(key.fingerprint, value)
                                  }
                                >
                                  <SelectTrigger
                                    className="h-8 w-[220px]"
                                    aria-label={t("workspaces.keys.workspace")}
                                  >
                                    <SelectValue />
                                  </SelectTrigger>
                                  <SelectContent>
                                    <SelectItem
                                      value={DEFAULT_WORKSPACE_OPTION}
                                    >
                                      {t("workspaces.keys.useDefault")}
                                    </SelectItem>
                                    {profiles.map((profile) => (
                                      <SelectItem
                                        key={profile.id}
                                        value={profile.id}
                                      >
                                        {profile.name || profile.id}
                                      </SelectItem>
                                    ))}
                                  </SelectContent>
                                </Select>
                                <p className="text-muted-foreground mt-1 max-w-[260px] font-mono text-[10px] break-all">
                                  {boundProfile
                                    ? (profiles.find(
                                        (profile) =>
                                          profile.id === boundProfile,
                                      )?.path ?? "")
                                    : data.default_workspace}
                                </p>
                              </td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </section>

              <section>
                <h3 className="text-sm font-semibold">
                  {t("workspaces.agents.title")}
                </h3>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("workspaces.agents.description")}
                </p>
                <div className="mt-3 overflow-x-auto rounded-xl border">
                  <table className="w-full text-left text-sm">
                    <thead className="bg-muted/40 text-muted-foreground text-xs">
                      <tr>
                        <th className="px-4 py-2.5 font-medium">
                          {t("workspaces.agents.agent")}
                        </th>
                        <th className="px-4 py-2.5 font-medium">
                          {t("workspaces.agents.model")}
                        </th>
                        <th className="px-4 py-2.5 font-medium">
                          {t("workspaces.agents.workspace")}
                        </th>
                        <th className="px-4 py-2.5 font-medium">
                          {t("workspaces.agents.binding")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {(data.agents ?? []).map((agent) => (
                        <tr key={agent.id} className="border-t align-top">
                          <td className="px-4 py-3">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-medium">{agent.id}</span>
                              {agent.name && (
                                <span className="text-muted-foreground text-xs">
                                  {agent.name}
                                </span>
                              )}
                              {agent.is_default && (
                                <Badge variant="secondary">
                                  {t("workspaces.agents.default")}
                                </Badge>
                              )}
                            </div>
                          </td>
                          <td className="px-4 py-3 font-mono text-xs">
                            {agent.model}
                          </td>
                          <td className="px-4 py-3 font-mono text-xs break-all">
                            {agent.workspace}
                          </td>
                          <td className="px-4 py-3">
                            {agent.profile_id ? (
                              <Badge variant="secondary">
                                {profileName(agent.profile_id)}
                              </Badge>
                            ) : (
                              <span className="text-muted-foreground text-xs">
                                {t("workspaces.agents.unbound")}
                              </span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </>
          )}
        </div>
      </div>

      <ProfileDialog
        open={dialogOpen}
        initial={editingProfile}
        existingIds={profiles.map((profile) => profile.id)}
        onClose={() => {
          setDialogOpen(false)
          setEditingProfile(null)
        }}
        onSubmit={handleSubmitProfile}
      />

      <AlertDialog
        open={deletingProfile !== null}
        onOpenChange={(open) => !open && setDeletingProfile(null)}
      >
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("workspaces.profiles.deleteTitle")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("workspaces.profiles.deleteDescription", {
                name: deletingProfile?.name || deletingProfile?.id,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setDeletingProfile(null)}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleDeleteProfile}
            >
              {t("workspaces.profiles.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
