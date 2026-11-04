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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import {
  getContentEditableCursor,
  getContentEditablePlainText,
  getMentionTagHtml,
  setContentEditableCursor,
} from '../lib/contenteditable-mentions'
import {
  getAtImageQuery,
  getImageMentionLabel,
  getPromptMentionParts,
  imageMentionMatches,
  insertImageMention,
} from '../lib/prompt-mentions'
import type { ImagePlaygroundReference } from '../types'

type MentionTextareaProps = {
  id?: string
  value: string
  references: ImagePlaygroundReference[]
  disabled?: boolean
  maxLength?: number
  placeholder?: string
  rows?: number
  onChange: (value: string) => void
}

/** Render the stored prompt (with markers) into editor HTML with mention chips. */
function promptToHtml(value: string, imageCount: number): string {
  const escape = (text: string) =>
    text
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll('\n', '<br>')
  return getPromptMentionParts(value, imageCount)
    .map((part) =>
      part.type === 'mention'
        ? getMentionTagHtml(getImageMentionLabel(part.imageIndex))
        : escape(part.text)
    )
    .join('')
}

export function MentionTextarea(props: MentionTextareaProps) {
  const { t } = useTranslation()
  const editorRef = useRef<HTMLDivElement | null>(null)
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const [query, setQuery] = useState<{ start: number; query: string } | null>(
    null
  )
  const references = props.references
  const imageCount = references.length

  // Re-sync editor HTML when the value or the reference set changes externally.
  useEffect(() => {
    const el = editorRef.current
    if (!el) return
    const plain = getContentEditablePlainText(el)
    if (plain === props.value) return
    el.innerHTML = promptToHtml(props.value, imageCount)
    setContentEditableCursor(el, props.value.length)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.value, imageCount])

  const matches =
    query === null
      ? []
      : references
          .map((ref, index) => ({ ref, index }))
          .filter((item) => imageMentionMatches(query.query, item.index))

  useEffect(() => {
    if (open) setActiveIndex(0)
  }, [open])

  function emitChange() {
    const el = editorRef.current
    if (!el) return
    let value = getContentEditablePlainText(el)
    if (props.maxLength !== undefined && value.length > props.maxLength) {
      value = value.slice(0, props.maxLength)
      el.innerHTML = promptToHtml(value, imageCount)
      setContentEditableCursor(el, value.length)
    }
    props.onChange(value)
  }

  function refreshQuery() {
    const el = editorRef.current
    if (!el || props.disabled) return
    const cursor = getContentEditableCursor(el)
    const text = getContentEditablePlainText(el)
    const at = getAtImageQuery(text, cursor, imageCount)
    setQuery(at)
    setOpen(at !== null && imageCount > 0)
  }

  function chooseImage(index: number) {
    const el = editorRef.current
    if (!el || query === null) return
    const text = getContentEditablePlainText(el)
    const cursor = getContentEditableCursor(el)
    const apply = insertImageMention(text, query.start, cursor, index)
    props.onChange(apply.prompt)
    setOpen(false)
    setQuery(null)
    // Reposition editor content + caret after React re-renders the value.
    requestAnimationFrame(() => {
      const node = editorRef.current
      if (!node) return
      node.innerHTML = promptToHtml(apply.prompt, imageCount)
      setContentEditableCursor(node, apply.cursor)
    })
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (open && matches.length > 0) {
      if (event.key === 'ArrowDown') {
        event.preventDefault()
        setActiveIndex((current) => (current + 1) % matches.length)
        return
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault()
        setActiveIndex((current) => (current - 1 + matches.length) % matches.length)
        return
      }
      if (event.key === 'Enter' || event.key === 'Tab') {
        event.preventDefault()
        chooseImage(matches[activeIndex].index)
        return
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        setOpen(false)
        setQuery(null)
        return
      }
    }
  }

  return (
    <div className='relative'>
      <div
        aria-disabled={props.disabled}
        aria-multiline='true'
        className={cn(
          'border-input bg-transparent dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50',
          'min-h-24 w-full rounded-md border px-3 py-2 text-sm shadow-xs transition-[color,box-shadow]',
          'whitespace-pre-wrap break-words outline-none focus-visible:ring-[3px]',
          '[&_.mention-tag]:bg-primary/10 [&_.mention-tag]:text-primary',
          '[&_.mention-tag]:mx-0.5 [&_.mention-tag]:inline-block [&_.mention-tag]:rounded [&_.mention-tag]:px-1 [&_.mention-tag]:text-xs [&_.mention-tag]:font-medium',
          props.disabled && 'cursor-not-allowed opacity-50'
        )}
        contentEditable={!props.disabled}
        id={props.id}
        onBlur={() => {
          // Delay so a click on the dropdown still registers.
          window.setTimeout(() => {
            setOpen(false)
            setQuery(null)
          }, 150)
          emitChange()
        }}
        onInput={() => {
          emitChange()
          refreshQuery()
        }}
        onKeyDown={handleKeyDown}
        onKeyUp={(event) => {
          if (
            ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(
              event.key
            )
          ) {
            refreshQuery()
          }
        }}
        onMouseUp={refreshQuery}
        ref={editorRef}
        role='textbox'
        style={{ minHeight: `${(props.rows ?? 4) * 1.5 + 1}rem` }}
        suppressContentEditableWarning
      />
      {props.value.length === 0 && (
        <p className='text-muted-foreground pointer-events-none absolute top-2 left-3 text-sm'>
          {props.placeholder}
        </p>
      )}
      {open && matches.length > 0 && (
        <div
          className='bg-popover text-popover-foreground absolute z-50 mt-1 max-h-60 w-56 overflow-auto rounded-md border p-1 shadow-md'
          onMouseDown={(event) => event.preventDefault()}
          role='listbox'
        >
          <div className='text-muted-foreground px-2 py-1 text-[11px]'>
            {t('Reference images')}
          </div>
          {matches.map((item, idx) => (
            <button
              aria-selected={idx === activeIndex}
              className={cn(
                'flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm',
                idx === activeIndex
                  ? 'bg-accent text-accent-foreground'
                  : 'hover:bg-accent/60'
              )}
              key={item.ref.id}
              onClick={() => chooseImage(item.index)}
              role='option'
              type='button'
            >
              <img
                alt=''
                className='size-6 rounded object-cover'
                src={item.ref.previewUrl}
              />
              <span>{getImageMentionLabel(item.index)}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
