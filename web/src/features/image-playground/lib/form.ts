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
import {
  DEFAULT_IMAGE_PLAYGROUND_FORM,
  IMAGE_PLAYGROUND_STORAGE_KEY,
} from '../constants'
import type { ImagePlaygroundForm } from '../types'

export function readStoredImagePlaygroundForm(): ImagePlaygroundForm {
  try {
    const raw = window.localStorage.getItem(IMAGE_PLAYGROUND_STORAGE_KEY)
    if (!raw) return { ...DEFAULT_IMAGE_PLAYGROUND_FORM }
    const parsed = JSON.parse(raw) as Partial<ImagePlaygroundForm>
    return normalizeImagePlaygroundForm(parsed)
  } catch {
    return { ...DEFAULT_IMAGE_PLAYGROUND_FORM }
  }
}

export function storeImagePlaygroundForm(form: ImagePlaygroundForm): void {
  try {
    window.localStorage.setItem(
      IMAGE_PLAYGROUND_STORAGE_KEY,
      JSON.stringify(form)
    )
  } catch {
    // Storage may be unavailable (private mode or quota); persistence is optional.
  }
}

export function normalizeImagePlaygroundForm(
  value: Partial<ImagePlaygroundForm>
): ImagePlaygroundForm {
  return {
    model: typeof value.model === 'string' ? value.model : '',
    group: typeof value.group === 'string' ? value.group : '',
    prompt: typeof value.prompt === 'string' ? value.prompt : '',
    size:
      typeof value.size === 'string'
        ? value.size
        : DEFAULT_IMAGE_PLAYGROUND_FORM.size,
    quality:
      typeof value.quality === 'string'
        ? value.quality
        : DEFAULT_IMAGE_PLAYGROUND_FORM.quality,
    background:
      typeof value.background === 'string'
        ? value.background
        : DEFAULT_IMAGE_PLAYGROUND_FORM.background,
    moderation:
      typeof value.moderation === 'string'
        ? value.moderation
        : DEFAULT_IMAGE_PLAYGROUND_FORM.moderation,
    outputFormat:
      typeof value.outputFormat === 'string'
        ? value.outputFormat
        : DEFAULT_IMAGE_PLAYGROUND_FORM.outputFormat,
    responseFormat:
      typeof value.responseFormat === 'string'
        ? value.responseFormat
        : DEFAULT_IMAGE_PLAYGROUND_FORM.responseFormat,
    stream:
      typeof value.stream === 'boolean'
        ? value.stream
        : DEFAULT_IMAGE_PLAYGROUND_FORM.stream,
    partialImages:
      typeof value.partialImages === 'number' &&
      Number.isFinite(value.partialImages)
        ? value.partialImages
        : DEFAULT_IMAGE_PLAYGROUND_FORM.partialImages,
    n: typeof value.n === 'number' && Number.isFinite(value.n) ? value.n : 1,
  }
}

export function buildImagePlaygroundFormData(
  form: ImagePlaygroundForm,
  referenceFiles: File[]
): FormData {
  const data = new FormData()
  data.append('model', form.model)
  data.append('group', form.group)
  data.append('prompt', form.prompt)
  data.append('size', form.size)
  data.append('quality', form.quality)
  data.append('background', form.background)
  data.append('moderation', form.moderation)
  data.append('output_format', form.outputFormat)
  data.append('response_format', form.responseFormat)
  data.append('stream', form.stream ? 'true' : 'false')
  if (form.stream) {
    data.append('partial_images', String(form.partialImages))
  }
  data.append('n', String(form.n))
  for (const file of referenceFiles) {
    data.append('references', file)
  }
  return data
}
