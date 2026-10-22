/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { IMAGE_PLAYGROUND_ENDPOINTS } from './constants'
import type { ImagePlaygroundConfig, ImagePlaygroundTask } from './types'

export async function getImagePlaygroundConfig(): Promise<ImagePlaygroundConfig> {
  const res = await api.get(IMAGE_PLAYGROUND_ENDPOINTS.CONFIG)
  requireServerSuccess(res.data)
  return res.data.data as ImagePlaygroundConfig
}

export async function listImagePlaygroundTasks(): Promise<
  ImagePlaygroundTask[]
> {
  const res = await api.get(IMAGE_PLAYGROUND_ENDPOINTS.TASKS)
  requireServerSuccess(res.data)
  return (res.data.data ?? []) as ImagePlaygroundTask[]
}

export async function createImagePlaygroundTask(
  payload: FormData
): Promise<ImagePlaygroundTask> {
  const res = await api.post(IMAGE_PLAYGROUND_ENDPOINTS.TASKS, payload)
  requireServerSuccess(res.data)
  return res.data.data as ImagePlaygroundTask
}

export async function deleteImagePlaygroundTask(id: string): Promise<void> {
  const res = await api.delete(`${IMAGE_PLAYGROUND_ENDPOINTS.TASKS}/${id}`)
  requireServerSuccess(res.data)
}

/** Same-origin URL of a stored image; the browser session authorizes it. */
export function imagePlaygroundAssetUrl(taskId: string, index: number): string {
  return `${IMAGE_PLAYGROUND_ENDPOINTS.TASKS}/${taskId}/assets/${index}`
}

/** Progressive preview URL; the frame bytes change, so callers append a nonce. */
export function imagePlaygroundPartialUrl(taskId: string): string {
  return `${IMAGE_PLAYGROUND_ENDPOINTS.TASKS}/${taskId}/partial`
}

export async function fetchImagePlaygroundBlob(url: string): Promise<Blob> {
  const response = await fetch(url, { credentials: 'same-origin' })
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`)
  }
  return response.blob()
}
