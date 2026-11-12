import { launcherFetch } from "@/api/http"

export interface SessionSummary {
  id: string
  title: string
  preview: string
  message_count: number
  created: string
  updated: string
}

export interface SessionDetail {
  id: string
  messages: {
    role: "user" | "assistant"
    content: string
    created_at?: string
    kind?: "normal" | "thought" | "tool_calls"
    model_name?: string
    media?: string[]
    attachments?: {
      type?: "image" | "audio" | "video" | "file"
      url: string
      filename?: string
      content_type?: string
    }[]
    tool_calls?: {
      id?: string
      type?: string
      function?: {
        name?: string
        arguments?: string
      }
      extra_content?: {
        tool_feedback_explanation?: string
      }
    }[]
  }[]
  summary: string
  created: string
  updated: string
}

function workspaceQuery(workspace?: string): string {
  return workspace ? `&workspace=${encodeURIComponent(workspace)}` : ""
}

export async function getSessions(
  offset: number = 0,
  limit: number = 20,
  workspace?: string,
): Promise<SessionSummary[]> {
  const params = new URLSearchParams({
    offset: offset.toString(),
    limit: limit.toString(),
  })

  const res = await launcherFetch(
    `/api/sessions?${params.toString()}${workspaceQuery(workspace)}`,
  )
  if (!res.ok) {
    throw new Error(`Failed to fetch sessions: ${res.status}`)
  }
  return res.json()
}

export async function getSessionHistory(
  id: string,
  workspace?: string,
): Promise<SessionDetail> {
  const res = await launcherFetch(
    `/api/sessions/${encodeURIComponent(id)}${workspace ? `?workspace=${encodeURIComponent(workspace)}` : ""}`,
  )
  if (!res.ok) {
    throw new Error(`Failed to fetch session ${id}: ${res.status}`)
  }
  return res.json()
}

export async function deleteSession(
  id: string,
  workspace?: string,
): Promise<void> {
  const res = await launcherFetch(
    `/api/sessions/${encodeURIComponent(id)}${workspace ? `?workspace=${encodeURIComponent(workspace)}` : ""}`,
    {
      method: "DELETE",
    },
  )
  if (!res.ok) {
    throw new Error(`Failed to delete session ${id}: ${res.status}`)
  }
}
