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

import {
  formatAspectRatio,
  formatImageSizeLabel,
  isValidImageSize,
  parseImageSize,
} from '../lib/size'

describe('parseImageSize', () => {
  it('parses valid width and height', () => {
    expect(parseImageSize('1024x1536')).toEqual({ width: 1024, height: 1536 })
  })

  it('trims surrounding whitespace before parsing', () => {
    expect(parseImageSize(' 2560x1440 ')).toEqual({
      width: 2560,
      height: 1440,
    })
  })

  it('returns null for unsupported formats', () => {
    expect(parseImageSize('auto')).toBeNull()
    expect(parseImageSize('1024*1024')).toBeNull()
    expect(parseImageSize('')).toBeNull()
  })
})

describe('isValidImageSize', () => {
  it('accepts auto and in-range sizes', () => {
    expect(isValidImageSize('auto')).toBe(true)
    expect(isValidImageSize('1024x1024')).toBe(true)
    expect(isValidImageSize('3840x2160')).toBe(true)
  })

  it('rejects sizes below the minimum edge', () => {
    expect(isValidImageSize('63x1024')).toBe(false)
    expect(isValidImageSize('1024x63')).toBe(false)
  })

  it('rejects sizes above the maximum edge', () => {
    expect(isValidImageSize('4097x1024')).toBe(false)
    expect(isValidImageSize('auto ')).toBe(false)
  })
})

describe('formatImageSizeLabel', () => {
  it('renders a multiplication sign for valid sizes', () => {
    expect(formatImageSizeLabel('1024x1536')).toBe('1024×1536')
  })

  it('keeps the original value when it cannot be parsed', () => {
    expect(formatImageSizeLabel('auto')).toBe('auto')
  })
})

describe('formatAspectRatio', () => {
  it('reduces common ratios', () => {
    expect(formatAspectRatio('1024x1024')).toBe('1:1')
    expect(formatAspectRatio('2560x1440')).toBe('16:9')
    expect(formatAspectRatio('1024x1536')).toBe('2:3')
  })

  it('returns an empty string for unparsable sizes', () => {
    expect(formatAspectRatio('auto')).toBe('')
  })
})
