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
  imagePlaygroundImageKey,
  parseImagePlaygroundImageKey,
} from '../lib/selection'

describe('imagePlaygroundImageKey', () => {
  it('combines the task id and the image index', () => {
    expect(imagePlaygroundImageKey('task-1', 2)).toBe('task-1:2')
  })
})

describe('parseImagePlaygroundImageKey', () => {
  it('restores the task id and index written by the key helper', () => {
    expect(
      parseImagePlaygroundImageKey(imagePlaygroundImageKey('a:b', 0))
    ).toEqual({ taskId: 'a:b', index: 0 })
  })

  it('rejects values without a task id or a numeric index', () => {
    expect(parseImagePlaygroundImageKey(':1')).toBeNull()
    expect(parseImagePlaygroundImageKey('task-1')).toBeNull()
    expect(parseImagePlaygroundImageKey('task-1:abc')).toBeNull()
    expect(parseImagePlaygroundImageKey('task-1:-1')).toBeNull()
  })
})
