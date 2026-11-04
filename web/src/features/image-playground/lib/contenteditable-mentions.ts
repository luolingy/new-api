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
import { getSelectedTextMentionLabel } from './prompt-mentions'

function getMentionTagTextLength(el: Element) {
  return el.textContent?.length ?? 0
}

function getNodeVisibleTextLength(node: Node): number {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent?.length ?? 0
  if (node instanceof HTMLElement && node.classList.contains('mention-tag')) {
    return getMentionTagTextLength(node)
  }
  return [...node.childNodes].reduce(
    (sum, child) => sum + getNodeVisibleTextLength(child),
    0
  )
}
function getVisibleOffsetBeforeNode(root: HTMLElement, target: Node): number {
  let offset = 0
  let found = false
  const walk = (node: Node) => {
    if (found) return
    if (node === target) {
      found = true
      return
    }
    if (node.nodeType === Node.TEXT_NODE) {
      offset += node.textContent?.length ?? 0
      return
    }
    if (node instanceof HTMLElement && node.classList.contains('mention-tag')) {
      offset += getMentionTagTextLength(node)
      return
    }
    node.childNodes.forEach(walk)
  }
  root.childNodes.forEach(walk)
  return offset
}

function getMentionTagForBoundary(root: HTMLElement, container: Node) {
  const el =
    container.nodeType === Node.ELEMENT_NODE
      ? (container as Element)
      : container.parentElement
  const tag = el?.closest('.mention-tag')
  return tag && root.contains(tag) ? tag : null
}

function getBoundaryOffsetInMention(
  tag: Element,
  container: Node,
  offset: number
) {
  try {
    const range = document.createRange()
    range.selectNodeContents(tag)
    range.setEnd(container, offset)
    return range.toString().length
  } catch {
    return getMentionTagTextLength(tag)
  }
}

function getContentEditableBoundaryOffset(
  root: HTMLElement,
  container: Node,
  offset: number,
  edge: 'start' | 'end',
  collapsed: boolean
): number {
  if (container === root) {
    let visibleOffset = 0
    for (const child of [...root.childNodes].slice(0, offset)) {
      visibleOffset += getNodeVisibleTextLength(child)
    }
    return visibleOffset
  }

  if (!root.contains(container)) {
    const position = root.compareDocumentPosition(container)
    if (position & Node.DOCUMENT_POSITION_PRECEDING) return 0
    if (position & Node.DOCUMENT_POSITION_FOLLOWING) {
      return root.textContent?.length ?? 0
    }
    return edge === 'start' ? 0 : (root.textContent?.length ?? 0)
  }

  const mentionTag = getMentionTagForBoundary(root, container)
  if (mentionTag) {
    const mentionStart = getVisibleOffsetBeforeNode(root, mentionTag)
    const mentionLength = getMentionTagTextLength(mentionTag)
    if (!collapsed) {
      return edge === 'start' ? mentionStart : mentionStart + mentionLength
    }
    const mentionOffset = getBoundaryOffsetInMention(mentionTag, container, offset)
    return mentionStart + (mentionOffset < mentionLength / 2 ? 0 : mentionLength)
  }

  if (container.nodeType === Node.TEXT_NODE) {
    return getVisibleOffsetBeforeNode(root, container) + offset
  }

  const element = container.nodeType === Node.ELEMENT_NODE ? (container as Element) : null
  if (element) {
    let visibleOffset = element === root ? 0 : getVisibleOffsetBeforeNode(root, element)
    for (const child of [...element.childNodes].slice(0, offset)) {
      visibleOffset += getNodeVisibleTextLength(child)
    }
    return visibleOffset
  }

  return root.textContent?.length ?? 0
}

/** Plain-text caret offset inside a contentEditable. */
export function getContentEditableCursor(el: HTMLElement): number {
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0) return el.textContent?.length ?? 0
  const range = sel.getRangeAt(0)
  if (!el.contains(range.startContainer)) return el.textContent?.length ?? 0
  return getContentEditableBoundaryOffset(
    el,
    range.startContainer,
    range.startOffset,
    'start',
    range.collapsed
  )
}

/** Plain-text selection range inside a contentEditable. */
export function getContentEditableSelection(el: HTMLElement): {
  start: number
  end: number
} {
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0) {
    const end = el.textContent?.length ?? 0
    return { start: end, end }
  }
  const range = sel.getRangeAt(0)
  const start = getContentEditableBoundaryOffset(
    el,
    range.startContainer,
    range.startOffset,
    'start',
    range.collapsed
  )
  const end = range.collapsed
    ? start
    : getContentEditableBoundaryOffset(
        el,
        range.endContainer,
        range.endOffset,
        'end',
        false
      )
  return { start, end }
}

/** Plain text of the editor; mention chips contribute their stored mention text. */
export function getContentEditablePlainText(el: HTMLElement): string {
  let text = ''
  const append = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      text += node.textContent ?? ''
      return
    }
    if (node instanceof HTMLElement && node.classList.contains('mention-tag')) {
      text += node.dataset.mentionText ?? node.textContent ?? ''
      return
    }
    node.childNodes.forEach(append)
  }
  el.childNodes.forEach(append)
  return text.replaceAll(/\r\n?/g, '\n')
}

function escapeHtml(text: string): string {
  return text
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}

/** HTML for a mention chip. mentionText may come from imported data and is escaped. */
export function getMentionTagHtml(
  text: string,
  mentionText = getSelectedTextMentionLabel(text)
): string {
  return `<span contenteditable="false" class="mention-tag" data-mention-text="${escapeHtml(
    mentionText
  )}">${escapeHtml(text)}</span>`
}

/** Move the caret to a plain-text offset inside a contentEditable. */
export function setContentEditableCursor(el: HTMLElement, offset: number): void {
  const sel = window.getSelection()
  if (!sel) return
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT)
  let remaining = offset
  let node: Text | null = null
  while (walker.nextNode()) {
    node = walker.currentNode as Text
    const mentionTag = node.parentElement?.closest('.mention-tag')
    if (mentionTag) {
      if (remaining <= node.length) {
        const range = document.createRange()
        if (remaining < node.length / 2) {
          range.setStartBefore(mentionTag)
        } else {
          range.setStartAfter(mentionTag)
        }
        range.collapse(true)
        sel.removeAllRanges()
        sel.addRange(range)
        return
      }
      remaining -= node.length
      continue
    }
    if (remaining <= node.length) {
      const range = document.createRange()
      range.setStart(node, remaining)
      range.collapse(true)
      sel.removeAllRanges()
      sel.addRange(range)
      return
    }
    remaining -= node.length
  }
  if (node) {
    const range = document.createRange()
    range.setStart(node, node.length)
    range.collapse(true)
    sel.removeAllRanges()
    sel.addRange(range)
  }
}
