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
  getAtImageQuery,
  getImageMentionLabel,
  getPromptMentionParts,
  getSelectedImageMentionLabel,
  imageMentionMatches,
  insertImageMention,
  replaceImageMentionsForApi,
  stripImageMentionMarkers,
} from '../lib/prompt-mentions'

const START = '\u2063'
const END = '\u2064'

describe('prompt-mentions', () => {
  it('builds mention labels', () => {
    expect(getImageMentionLabel(0)).toBe('@图1')
    expect(getSelectedImageMentionLabel(0)).toBe(`${START}@图1${END}`)
  })

  it('detects an @ query before the cursor', () => {
    expect(getAtImageQuery('hello @', 7, 2)).toEqual({ start: 6, query: '' })
    expect(getAtImageQuery('hello @图', 8, 2)).toEqual({ start: 6, query: '图' })
    expect(getAtImageQuery('hello @2', 8, 2)).toEqual({ start: 6, query: '2' })
  })

  it('returns null when not an active query', () => {
    expect(getAtImageQuery('hello world', 11, 2)).toBeNull()
    expect(getAtImageQuery('@foo bar', 8, 2)).toBeNull()
    expect(getAtImageQuery('@', 1, 0)).toBeNull()
  })

  it('matches mention queries by index or label', () => {
    expect(imageMentionMatches('', 0)).toBe(true)
    expect(imageMentionMatches('1', 0)).toBe(true)
    expect(imageMentionMatches('2', 0)).toBe(false)
    expect(imageMentionMatches('图2', 1)).toBe(true)
  })

  it('inserts a mention chip and returns the caret position', () => {
    const result = insertImageMention('a @b', 2, 4, 0)
    expect(result.prompt).toBe(`a ${START}@图1${END}`)
    expect(result.cursor).toBe(2 + '@图1'.length)
  })

  it('splits stored prompt into text and mention parts', () => {
    const prompt = `before ${START}@图1${END} after`
    const parts = getPromptMentionParts(prompt, 1)
    expect(parts).toHaveLength(3)
    expect(parts[0]).toEqual({ type: 'text', text: 'before ' })
    expect(parts[1]).toMatchObject({ type: 'mention', imageIndex: 0 })
    expect(parts[2]).toEqual({ type: 'text', text: ' after' })
  })

  it('collapses mentions into [image N] for the API', () => {
    const prompt = `a ${START}@图1${END} b ${START}@图2${END}`
    expect(replaceImageMentionsForApi(prompt)).toBe('a [image 1] b [image 2]')
  })

  it('drops out-of-range mentions when the count is known', () => {
    const prompt = `a ${START}@图2${END}`
    expect(replaceImageMentionsForApi(prompt, 1)).toBe('a ')
  })

  it('strips markers without dropping visible text', () => {
    const prompt = `a ${START}@图1${END} b`
    expect(stripImageMentionMarkers(prompt)).toBe('a @图1 b')
  })
})
