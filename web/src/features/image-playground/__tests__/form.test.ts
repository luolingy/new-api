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
  buildImagePlaygroundFormData,
  normalizeImagePlaygroundForm,
} from '../lib/form'

function readFormData(data: FormData) {
  return {
    stream: data.get('stream'),
    partialImages: data.get('partial_images'),
    responseFormat: data.get('response_format'),
    outputFormat: data.get('output_format'),
  }
}

describe('buildImagePlaygroundFormData', () => {
  it('sends stream frames and the base64 response format when streaming is enabled', () => {
    const data = buildImagePlaygroundFormData(
      {
        ...DEFAULT_IMAGE_PLAYGROUND_FORM,
        model: 'gpt-image-1',
        prompt: 'a cat',
        stream: true,
        partialImages: 3,
        responseFormat: 'b64_json',
      },
      []
    )

    const fields = readFormData(data)
    expect(fields.stream).toBe('true')
    expect(fields.partialImages).toBe('3')
    expect(fields.responseFormat).toBe('b64_json')
  })

  it('omits the frame count and streams nothing when streaming is disabled', () => {
    const data = buildImagePlaygroundFormData(
      {
        ...DEFAULT_IMAGE_PLAYGROUND_FORM,
        model: 'gpt-image-1',
        prompt: 'a cat',
        stream: false,
        responseFormat: 'url',
      },
      []
    )

    const fields = readFormData(data)
    expect(fields.stream).toBe('false')
    expect(fields.partialImages).toBeNull()
    expect(fields.responseFormat).toBe('url')
  })

  it('appends every reference image as a multipart file part', () => {
    const first = new File(['a'], 'a.png', { type: 'image/png' })
    const second = new File(['b'], 'b.png', { type: 'image/png' })

    const data = buildImagePlaygroundFormData(
      {
        ...DEFAULT_IMAGE_PLAYGROUND_FORM,
        model: 'gpt-image-1',
        prompt: 'a cat',
      },
      [first, second]
    )

    expect(data.getAll('references')).toHaveLength(2)
  })
})

describe('normalizeImagePlaygroundForm', () => {
  it('fills streaming defaults when the stored form predates the options', () => {
    const normalized = normalizeImagePlaygroundForm({ prompt: 'a cat' })

    expect(normalized.stream).toBe(false)
    expect(normalized.partialImages).toBe(
      DEFAULT_IMAGE_PLAYGROUND_FORM.partialImages
    )
    expect(normalized.responseFormat).toBe(
      DEFAULT_IMAGE_PLAYGROUND_FORM.responseFormat
    )
  })

  it('keeps stored streaming values intact', () => {
    const normalized = normalizeImagePlaygroundForm({
      stream: true,
      partialImages: 1,
      responseFormat: 'url',
    })

    expect(normalized.stream).toBe(true)
    expect(normalized.partialImages).toBe(1)
    expect(normalized.responseFormat).toBe('url')
  })
})
