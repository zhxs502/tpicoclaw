import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import type { WorkspaceProfileInput } from "@/api/workspaces"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

interface ProfileDialogProps {
  open: boolean
  initial: WorkspaceProfileInput | null
  existingIds: string[]
  onClose: () => void
  onSubmit: (profile: WorkspaceProfileInput) => void
}

function uniqueProfileID(name: string, existingIDs: string[]): string {
  const base =
    name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 48) || "workspace"
  const used = new Set(existingIDs.map((id) => id.toLowerCase()))
  let candidate = base
  let suffix = 2
  while (used.has(candidate)) {
    candidate = `${base}-${suffix}`
    suffix += 1
  }
  return candidate
}

export function ProfileDialog({
  open,
  initial,
  existingIds,
  onClose,
  onSubmit,
}: ProfileDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState("")
  const [path, setPath] = useState("")
  const [error, setError] = useState("")

  useEffect(() => {
    if (!open) return
    setName(initial?.name ?? "")
    setPath(initial?.path ?? "")
    setError("")
  }, [open, initial])

  const handleSubmit = () => {
    const trimmedName = name.trim()
    const trimmedPath = path.trim()
    if (!trimmedName) {
      setError(t("workspaces.dialog.nameRequired"))
      return
    }
    if (!trimmedPath) {
      setError(t("workspaces.dialog.pathRequired"))
      return
    }
    const id = initial?.id || uniqueProfileID(trimmedName, existingIds)
    onSubmit({
      id,
      name: trimmedName,
      path: trimmedPath,
    })
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {initial
              ? t("workspaces.dialog.editTitle")
              : t("workspaces.dialog.addTitle")}
          </DialogTitle>
          <DialogDescription>
            {t("workspaces.dialog.description")}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="workspace-profile-name">
              {t("workspaces.dialog.name")}
            </Label>
            <Input
              id="workspace-profile-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={t("workspaces.dialog.namePlaceholder")}
              autoFocus
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="workspace-profile-path">
              {t("workspaces.dialog.path")}
            </Label>
            <Input
              id="workspace-profile-path"
              value={path}
              onChange={(event) => setPath(event.target.value)}
              placeholder={t("workspaces.dialog.pathPlaceholder")}
              className="font-mono text-xs"
            />
            <p className="text-muted-foreground text-xs">
              {t("workspaces.dialog.pathHint")}
            </p>
          </div>

          {error && <p className="text-destructive text-sm">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button onClick={handleSubmit}>
            {initial
              ? t("workspaces.dialog.save")
              : t("workspaces.dialog.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
