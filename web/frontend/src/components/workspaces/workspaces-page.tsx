import {
  IconAlertTriangle,
  IconEdit,
  IconFolder,
  IconLoader2,
  IconPlus,
  IconShieldLock,
  IconTrash,
} from "@tabler/icons-react"
import { useCallback, useEffect, useRef, useState } from "react"
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
import { Switch } from "@/components/ui/switch"
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"
import { refreshGatewayState } from "@/store/gateway"

import { ProfileDialog } from "./profile-dialog"

function toInput(profile: WorkspaceProfile): WorkspaceProfileInput {
  return {
    id: profile.id,
    name: profile.name ?? profile.id,
    path: profile.path,
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
            profiles: nextProfiles.map((profile) => ({ ...profile })),
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

  const handleToggle = (value: boolean) => {
    setEnabled(value)
    setDirty(true)
    void persist(value, profiles)
  }

  const handleSubmitProfile = (profile: WorkspaceProfileInput) => {
    const exists = profiles.some((item) => item.id === profile.id)
    const nextProfiles = exists
      ? profiles.map((item) => (item.id === profile.id ? profile : item))
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
            onClick={handleSave}
            disabled={!dirty || saving || loading}
          >
            {saving && <IconLoader2 className="size-4 animate-spin" />}
            {t("common.save")}
          </Button>
        </div>
      </PageHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-10 sm:px-6">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 pt-2">
          <div>
            <p className="text-muted-foreground text-sm">
              {t("workspaces.description")}
            </p>
            <p className="text-muted-foreground/80 mt-1 text-xs">
              {t("workspaces.applyHint")}
            </p>
          </div>

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
                  <Switch checked={enabled} onCheckedChange={handleToggle} />
                </div>
                <div className="bg-muted/40 mt-4 rounded-lg p-3 text-xs">
                  <p className="text-muted-foreground">
                    {t("workspaces.defaultWorkspace")}
                  </p>
                  <p className="mt-1 font-mono break-all">
                    {data.default_workspace}
                  </p>
                </div>
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
                      </div>
                    ))}
                  </div>
                )}
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
