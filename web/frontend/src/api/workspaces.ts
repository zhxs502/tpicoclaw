import { launcherFetch } from "@/api/http"

// API client for named workspace profiles.

export interface WorkspaceProfile {
  id: string
  name?: string
  path: string
}

export interface WorkspacesResponse {
  enabled: boolean
  default_workspace: string
  profiles: WorkspaceProfile[]
}

export interface WorkspaceProfileInput {
  id: string
  name: string
  path: string
}

export interface WorkspacesUpdate {
  enabled: boolean
  profiles: WorkspaceProfileInput[]
}

const BASE_URL = ""

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await launcherFetch(`${BASE_URL}${path}`, options)
  if (!res.ok) {
    let detail = ""
    try {
      const body = (await res.json()) as {
        errors?: string[]
        error?: string
      }
      if (Array.isArray(body.errors) && body.errors.length > 0) {
        detail = body.errors.join("; ")
      } else if (typeof body.error === "string") {
        detail = body.error
      }
    } catch {
      // ignore non-JSON errors
    }
    if (!detail) {
      try {
        detail = await res.text()
      } catch {
        // ignore
      }
    }
    throw new Error(detail || `API error: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function getWorkspaces(): Promise<WorkspacesResponse> {
  return request<WorkspacesResponse>("/api/workspaces")
}

export async function updateWorkspaces(
  input: WorkspacesUpdate,
): Promise<WorkspacesResponse> {
  return request<WorkspacesResponse>("/api/workspaces", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  })
}
