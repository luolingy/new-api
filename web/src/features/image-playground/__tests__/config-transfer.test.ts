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
import { describe, expect, it } from 'vitest'

import { DEFAULT_IMAGE_PLAYGROUND_FORM } from '../constants'
import {
  buildConfigExport,
  configExportFileName,
  parseConfigImport,
  serializeConfigExport,
} from '../lib/config-transfer'
import type { ImagePlaygroundForm } from '../types'

function sampleForm(): ImagePlaygroundForm {
  return {
    ...DEFAULT_IMAGE_PLAYGROUND_FORM,
    model: 'gpt-image-1',
    group: 'default',
    prompt: 'a cat',
    size: '1024x1024',
    quality: 'high',
    n: 3,
  }
}

describe('config-transfer', () => {
  it('wraps the form in a versioned export payload', () => {
    const exported = buildConfigExport(sampleForm())
    expect(exported.type).toBe('new-api-image-playground-config')
    expect(exported.version).toBe(1)
    expect(exported.form.n).toBe(3)
    expect(typeof exported.exportedAt).toBe('number')
  })

  it('round-trips through serialize/parse', () => {
    const form = sampleForm()
    const result = parseConfigImport(serializeConfigExport(form))
    expect(result).toEqual(form)
  })

  it('accepts a bare form object without the wrapper', () => {
    const form = sampleForm()
    const result = parseConfigImport(JSON.stringify(form))
    expect(result.model).toBe('gpt-image-1')
    expect(result.n).toBe(3)
  })

  it('normalizes missing/invalid fields to defaults', () => {
    const result = parseConfigImport(JSON.stringify({ model: 'x', n: 'bad' }))
    expect(result.model).toBe('x')
    expect(result.n).toBe(1)
    expect(result.size).toBe(DEFAULT_IMAGE_PLAYGROUND_FORM.size)
  })

  it('throws on non-object payloads', () => {
    expect(() => parseConfigImport('[]')).toThrow()
    expect(() => parseConfigImport('123')).toThrow()
  })

  it('produces a timestamped file name', () => {
    const name = configExportFileName(new Date(2026, 0, 2, 3, 4, 5))
    expect(name).toBe('image-playground-config-20260102-030405.json')
  })
})
