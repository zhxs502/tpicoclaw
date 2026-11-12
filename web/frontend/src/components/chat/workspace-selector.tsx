import { IconFolder } from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import type { WorkspaceProfile } from "@/api/workspaces"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

const DEFAULT_WORKSPACE = "__default__"

interface WorkspaceSelectorProps {
  value: string
  workspaces: WorkspaceProfile[]
  disabled?: boolean
  onValueChange: (workspaceId: string) => void
}

export function WorkspaceSelector({
  value,
  workspaces,
  disabled = false,
  onValueChange,
}: WorkspaceSelectorProps) {
  const { t } = useTranslation()

  return (
    <Select
      value={value || DEFAULT_WORKSPACE}
      onValueChange={(next) =>
        onValueChange(next === DEFAULT_WORKSPACE ? "" : next)
      }
      disabled={disabled}
    >
      <SelectTrigger
        size="sm"
        className="text-muted-foreground hover:text-foreground focus-visible:border-input h-8 max-w-[160px] min-w-[80px] bg-transparent shadow-none focus-visible:ring-0 sm:max-w-[220px]"
        aria-label={t("chat.workspace")}
      >
        <IconFolder className="size-3.5 shrink-0 opacity-70" />
        <SelectValue placeholder={t("chat.workspaceDefault")} />
      </SelectTrigger>
      <SelectContent position="popper" align="start">
        <SelectItem value={DEFAULT_WORKSPACE}>
          {t("chat.workspaceDefault")}
        </SelectItem>
        {workspaces.map((workspace) => (
          <SelectItem key={workspace.id} value={workspace.id}>
            {workspace.name || workspace.id}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
