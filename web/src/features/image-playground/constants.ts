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
import type { ImagePlaygroundForm } from './types'

export const IMAGE_PLAYGROUND_ENDPOINTS = {
  CONFIG: '/api/image-playground/config',
  TASKS: '/api/image-playground/tasks',
} as const

export const IMAGE_PLAYGROUND_STORAGE_KEY = 'image-playground-form'

export const DEFAULT_IMAGE_PLAYGROUND_FORM: ImagePlaygroundForm = {
  model: '',
  group: '',
  prompt: '',
  size: 'auto',
  quality: 'auto',
  background: 'auto',
  moderation: 'auto',
  outputFormat: 'png',
  responseFormat: 'b64_json',
  stream: false,
  partialImages: 2,
  n: 1,
}

export const IMAGE_PLAYGROUND_RESPONSE_FORMATS = ['b64_json', 'url'] as const

export const IMAGE_PLAYGROUND_PARTIAL_IMAGE_OPTIONS = [0, 1, 2, 3] as const

export const IMAGE_PLAYGROUND_QUALITY_OPTIONS = [
  'auto',
  'low',
  'medium',
  'high',
] as const

export const IMAGE_PLAYGROUND_BACKGROUND_OPTIONS = [
  'auto',
  'transparent',
  'opaque',
] as const

export const IMAGE_PLAYGROUND_MODERATION_OPTIONS = ['auto', 'low'] as const

export const IMAGE_PLAYGROUND_OUTPUT_FORMATS = ['png', 'jpeg', 'webp'] as const

export const IMAGE_PLAYGROUND_POLL_INTERVAL_MS = 3000

export const IMAGE_PLAYGROUND_ACTIVE_STATUSES = ['queued', 'running'] as const

export const IMAGE_PLAYGROUND_MAX_REFERENCE_BYTES = 10 * 1024 * 1024

export const IMAGE_PLAYGROUND_MAX_PROMPT_LENGTH = 4000
