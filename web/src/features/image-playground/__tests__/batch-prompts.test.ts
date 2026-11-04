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

import { runWithConcurrency, splitBatchPrompts } from '../lib/batch-prompts'

describe('splitBatchPrompts', () => {
  it('keeps a single prompt intact', () => {
    expect(splitBatchPrompts('a cat')).toEqual(['a cat'])
  })

  it('keeps a single blank line inside one prompt', () => {
    expect(splitBatchPrompts('line one\n\nline two')).toEqual([
      'line one\n\nline two',
    ])
  })

  it('splits on two or more blank lines', () => {
    expect(splitBatchPrompts('first\n\n\nsecond\n\n\nthird')).toEqual([
      'first',
      'second',
      'third',
    ])
  })

  it('trims items and drops empty groups', () => {
    expect(splitBatchPrompts('  a  \n\n\n  \n\n\n b ')).toEqual(['a', 'b'])
  })

  it('returns an empty list for blank input', () => {
    expect(splitBatchPrompts('   ')).toEqual([])
  })
})

describe('runWithConcurrency', () => {
  it('visits every item exactly once', async () => {
    const seen: number[] = []
    await runWithConcurrency([1, 2, 3, 4, 5], 2, async (item) => {
      seen.push(item)
    })
    expect([...seen].sort((a, b) => a - b)).toEqual([1, 2, 3, 4, 5])
  })

  it('never exceeds the concurrency limit', async () => {
    let active = 0
    let peak = 0
    await runWithConcurrency([1, 2, 3, 4, 5, 6], 2, async () => {
      active += 1
      peak = Math.max(peak, active)
      await Promise.resolve()
      active -= 1
    })
    expect(peak).toBeLessThanOrEqual(2)
  })

  it('stops launching new items once told to stop', async () => {
    const visited: number[] = []
    await runWithConcurrency(
      [1, 2, 3, 4, 5],
      1,
      async (item) => {
        visited.push(item)
      },
      () => visited.length >= 2
    )
    expect(visited).toEqual([1, 2])
  })
})
