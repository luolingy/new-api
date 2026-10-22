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
  clampCropOffset,
  computeCropRect,
  coverScale,
} from '../lib/avatar-crop'

describe('coverScale', () => {
  it('uses the height ratio for a wide image so the viewport stays covered', () => {
    expect(coverScale({ width: 400, height: 200 }, 288)).toBe(1.44)
  })

  it('uses the width ratio for a tall image', () => {
    expect(coverScale({ width: 200, height: 400 }, 288)).toBe(1.44)
  })

  it('falls back to a neutral scale for a degenerate image', () => {
    expect(coverScale({ width: 0, height: 0 }, 288)).toBe(1)
  })
})

describe('clampCropOffset', () => {
  it('clamps a pan beyond the available overflow', () => {
    expect(clampCropOffset(500, 576, 288)).toBe(144)
    expect(clampCropOffset(-500, 576, 288)).toBe(-144)
  })

  it('keeps an offset that stays inside the overflow', () => {
    expect(clampCropOffset(30, 576, 288)).toBe(30)
  })

  it('forces a zero offset when the image exactly matches the viewport', () => {
    expect(clampCropOffset(50, 288, 288)).toBe(0)
  })
})

describe('computeCropRect', () => {
  it('centers a wide image at zoom 1 and fills the viewport height', () => {
    const rect = computeCropRect(
      { width: 400, height: 200 },
      { size: 288, zoom: 1, offsetX: 0, offsetY: 0 }
    )

    expect(rect).toEqual({ x: -144, y: 0, width: 576, height: 288 })
  })

  it('doubles the drawn size at zoom 2', () => {
    const rect = computeCropRect(
      { width: 400, height: 200 },
      { size: 288, zoom: 2, offsetX: 0, offsetY: 0 }
    )

    expect(rect.width).toBe(1152)
    expect(rect.height).toBe(576)
    expect(rect.x).toBe(-432)
    expect(rect.y).toBe(-144)
  })

  it('clamps a drag so empty space never appears inside the viewport', () => {
    const rect = computeCropRect(
      { width: 400, height: 200 },
      { size: 288, zoom: 1, offsetX: 10_000, offsetY: 0 }
    )

    expect(rect.x).toBe(0)
    expect(rect.x + rect.width).toBe(576)
  })
})
