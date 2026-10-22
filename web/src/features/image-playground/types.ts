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
export type ImagePlaygroundStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'

export type ImagePlaygroundImage = {
  index: number
  url: string
  mime_type: string
  revised_prompt?: string
}

export type ImagePlaygroundPartial = {
  index: number
  updated_at: number
  url: string
}

export type ImagePlaygroundTask = {
  id: string
  status: ImagePlaygroundStatus
  prompt: string
  model: string
  group: string
  size: string
  quality: string
  background: string
  moderation: string
  output_format: string
  stream: boolean
  partial_images: number
  response_format: string
  n: number
  reference_count: number
  images: ImagePlaygroundImage[]
  partial: ImagePlaygroundPartial | null
  error: string
  created_at: number
  started_at: number
  finished_at: number
}

export type ImagePlaygroundConfig = {
  enabled: boolean
  globally_enabled: boolean
  restricted: boolean
  message: string
  max_reference_images: number
  max_images_per_task: number
  allowed_models: string[]
  retention_days: number
}

export type ImagePlaygroundForm = {
  model: string
  group: string
  prompt: string
  size: string
  quality: string
  background: string
  moderation: string
  outputFormat: string
  responseFormat: string
  stream: boolean
  partialImages: number
  n: number
}

export type ImagePlaygroundReference = {
  id: string
  file: File
  previewUrl: string
}
