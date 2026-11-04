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
/**
 * @-mention support for reference images inside the prompt.
 *
 * The stored prompt carries invisible marker characters around every mention so
 * the editor can render them as non-editable "chips" while the plain text stays
 * reversible. Before sending to the backend the markers are collapsed into
 * `[image N]` placeholders (the backend only stores/forwards plain prompt text).
 *
 *   \u2063  mention start marker
 *   \u2064  mention end marker
 */
const MENTION_START = '\u2063'
const MENTION_END = '\u2064'
const SELECTED_IMAGE_MENTION_RE = /\u2063@图(\d+)\u2064/g

export interface AtImageQuery {
  start: number
  query: string
}

export function getImageMentionLabel(index: number): string {
  return `@图${index + 1}`
}

export function getSelectedTextMentionLabel(text: string): string {
  return `${MENTION_START}${text}${MENTION_END}`
}

export function getSelectedImageMentionLabel(index: number): string {
  return getSelectedTextMentionLabel(getImageMentionLabel(index))
}

/** Remove every marker so the prompt can be handled as plain text. */
export function stripImageMentionMarkers(prompt: string): string {
  return prompt.replaceAll(/[\u2063\u2064]/g, '')
}

/**
 * Detect an unfinished `@…` query just before the cursor. Returns null when the
 * cursor is not inside a mention token, or when there are no images to mention.
 */
export function getAtImageQuery(
  prompt: string,
  cursor: number,
  imageCount: number
): AtImageQuery | null {
  if (imageCount <= 0) return null
  const beforeCursor = prompt.slice(0, cursor)
  const atIndex = beforeCursor.lastIndexOf('@')
  if (atIndex < 0) return null
  const query = beforeCursor.slice(atIndex + 1)
  if (/\s/.test(query)) return null
  return { start: atIndex, query }
}

/** Whether the current @-query should list the image at `index`. */
export function imageMentionMatches(query: string, index: number): boolean {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  const oneBasedIndex = String(index + 1)
  const label = `图${oneBasedIndex}`
  return oneBasedIndex.includes(normalized) || label.toLowerCase().includes(normalized)
}

/** Insert a mention chip in place of the `[start, cursor)` @-query range. */
export function insertImageMention(
  prompt: string,
  start: number,
  cursor: number,
  imageIndex: number
): { prompt: string; cursor: number } {
  const mention = getSelectedImageMentionLabel(imageIndex)
  const visibleMention = getImageMentionLabel(imageIndex)
  const nextPrompt = `${prompt.slice(0, start)}${mention}${prompt.slice(cursor)}`
  return { prompt: nextPrompt, cursor: start + visibleMention.length }
}

export type PromptMentionPart =
  | { type: 'text'; text: string }
  | { type: 'mention'; text: string; imageIndex: number }

/** Split a stored prompt into plain-text and mention parts for rendering. */
export function getPromptMentionParts(
  prompt: string,
  imageCount: number
): PromptMentionPart[] {
  const parts: PromptMentionPart[] = []
  let lastIndex = 0
  for (const match of prompt.matchAll(SELECTED_IMAGE_MENTION_RE)) {
    if (match.index == null) continue
    const index = Number(match[1]) - 1
    if (index < 0 || index >= imageCount) continue
    if (match.index > lastIndex) {
      parts.push({
        type: 'text',
        text: stripImageMentionMarkers(prompt.slice(lastIndex, match.index)),
      })
    }
    parts.push({ type: 'mention', text: match[1], imageIndex: index })
    lastIndex = match.index + match[0].length
  }
  if (lastIndex < prompt.length) {
    parts.push({
      type: 'text',
      text: stripImageMentionMarkers(prompt.slice(lastIndex)),
    })
  }
  return parts.length > 0
    ? parts
    : [{ type: 'text', text: stripImageMentionMarkers(prompt) }]
}

/** Convert a stored prompt into the plain text sent to the backend/API. */
export function replaceImageMentionsForApi(
  prompt: string,
  imageCount?: number
): string {
  return prompt.replaceAll(SELECTED_IMAGE_MENTION_RE, (_text, n) => {
    const index = Number(n) - 1
    // A mention of an image that is no longer attached carries no meaning for
    // the upstream request, so it is dropped instead of leaking marker text.
    if (imageCount != null && (index < 0 || index >= imageCount)) {
      return ''
    }
    return `[image ${n}]`
  })
}
