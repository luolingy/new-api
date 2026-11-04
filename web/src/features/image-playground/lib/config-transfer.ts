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
import { DEFAULT_IMAGE_PLAYGROUND_FORM } from '../constants'
import type { ImagePlaygroundConfigExport, ImagePlaygroundForm } from '../types'
import { normalizeImagePlaygroundForm } from './form'

export const IMAGE_PLAYGROUND_CONFIG_EXPORT_VERSION = 1

/** Everything the playground remembers about the user's generation setup. */
export function buildConfigExport(
  form: ImagePlaygroundForm
): ImagePlaygroundConfigExport {
  return {
    type: 'new-api-image-playground-config',
    version: IMAGE_PLAYGROUND_CONFIG_EXPORT_VERSION,
    exportedAt: Date.now(),
    form: { ...form },
  }
}

export function serializeConfigExport(form: ImagePlaygroundForm): string {
  return JSON.stringify(buildConfigExport(form), null, 2)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * Parse an exported config file back into a normalized form. Tolerates a bare
 * form object (no wrapper) so users can hand-edit the JSON. Unknown or invalid
 * fields fall back to defaults via normalizeImagePlaygroundForm.
 */
export function parseConfigImport(raw: string): ImagePlaygroundForm {
  const parsed: unknown = JSON.parse(raw)
  if (!isRecord(parsed)) {
    throw new Error('invalid-config')
  }
  const candidate =
    parsed.type === 'new-api-image-playground-config' && isRecord(parsed.form)
      ? parsed.form
      : parsed
  return normalizeImagePlaygroundForm(candidate as Partial<ImagePlaygroundForm>)
}

/** Suggested filename for the exported config file. */
export function configExportFileName(now: Date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp =
    `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}` +
    `-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
  return `image-playground-config-${stamp}.json`
}

/** Default used when an import is missing optional fields. */
export const CONFIG_IMPORT_FALLBACK = DEFAULT_IMAGE_PLAYGROUND_FORM
