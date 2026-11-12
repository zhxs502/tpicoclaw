import { createFileRoute } from "@tanstack/react-router"

import { WorkspacesPage } from "@/components/workspaces/workspaces-page"

export const Route = createFileRoute("/workspaces")({
  component: WorkspacesPage,
})
