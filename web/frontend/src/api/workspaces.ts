import { launcherFetch } from "@/api/http"

// API client for workspace isolation (api key -> workspace path bindings).

export interface WorkspaceProfile {
  id: string
  name?: string
  path: string
  restrict_to_workspace?: boolean
  allow_read_outside_workspace?: boolean
  api_key_fingerprints: string[]
  key_count: number
}

export interface WorkspaceKey {
  model_name: string
  provider: string
  model: string
  key_index: number
  key_mask: string
  fingerprint: string
  profile_id?: string
  is_default_model: boolean
}

export interface WorkspaceAgent {
  id: string
  name?: string
  model: string
  workspace: string
  profile_id?: string
  is_default: boolean
  restrict_to_workspace: boolean
}

export interface WorkspacesResponse {
  enabled: boolean
  default_workspace: string
  default_model: string
  active_workspace: string
  active_profile_id?: string
  profiles: WorkspaceProfile[]
  keys: WorkspaceKey[]
  agents: WorkspaceAgent[]
  warnings?: string[]
}

export interface WorkspaceProfileInput {
  id: string
  name: string
  path: string
  restrict_to_workspace?: boolean
  allow_read_outside_workspace?: boolean
  api_key_fingerprints: string[]
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
