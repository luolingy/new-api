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
import { ImagePlus, X } from 'lucide-react'
import { useRef, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { IMAGE_PLAYGROUND_MAX_REFERENCE_BYTES } from '../constants'
import type { ImagePlaygroundReference } from '../types'

type ReferenceUploaderProps = {
  references: ImagePlaygroundReference[]
  max: number
  disabled?: boolean
  onChange: (references: ImagePlaygroundReference[]) => void
}

export function ReferenceUploader(props: ReferenceUploaderProps) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const canAddMore = props.references.length < props.max

  function acceptFiles(files: File[]) {
    const accepted: ImagePlaygroundReference[] = []
    for (const file of files) {
      if (props.references.length + accepted.length >= props.max) {
        toast.error(
          t('You can add at most {{count}} reference images', {
            count: props.max,
          })
        )
        break
      }
      if (!file.type.startsWith('image/')) {
        toast.error(t('Only image files can be used as reference images'))
        continue
      }
      if (file.size > IMAGE_PLAYGROUND_MAX_REFERENCE_BYTES) {
        toast.error(t('Each reference image must be smaller than 10 MB'))
        continue
      }
      accepted.push({
        id: `${file.name}-${file.lastModified}-${accepted.length}`,
        file,
        previewUrl: URL.createObjectURL(file),
      })
    }
    if (accepted.length > 0) {
      props.onChange([...props.references, ...accepted])
    }
  }

  function handleFilesRemoved(target: ImagePlaygroundReference) {
    URL.revokeObjectURL(target.previewUrl)
    props.onChange(props.references.filter((item) => item.id !== target.id))
  }

  function handleDrop(event: DragEvent<HTMLElement>) {
    event.preventDefault()
    if (props.disabled || !canAddMore) return
    acceptFiles([...event.dataTransfer.files])
  }

  return (
    <div className='space-y-2'>
      <div className='flex flex-wrap gap-2'>
        {props.references.map((reference) => (
          <div
            key={reference.id}
            className='group border-border bg-muted/40 relative size-16 overflow-hidden rounded-lg border'
          >
            <img
              alt={reference.file.name}
              className='size-full object-cover'
              src={reference.previewUrl}
            />
            <Button
              aria-label={t('Remove reference image')}
              className='bg-background/90 hover:bg-background absolute top-0.5 right-0.5 size-5 rounded-full opacity-90 shadow-sm'
              disabled={props.disabled}
              onClick={() => handleFilesRemoved(reference)}
              size='icon-sm'
              type='button'
              variant='ghost'
            >
              <X className='size-3' />
            </Button>
          </div>
        ))}
        {canAddMore && (
          <button
            className={cn(
              'border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
              'flex size-16 flex-col items-center justify-center gap-1 rounded-lg border border-dashed transition-colors',
              'focus-visible:ring-ring/50 focus-visible:ring-2 focus-visible:outline-none',
              props.disabled && 'pointer-events-none opacity-50'
            )}
            disabled={props.disabled}
            onClick={() => inputRef.current?.click()}
            onDragOver={(event) => event.preventDefault()}
            onDrop={handleDrop}
            type='button'
          >
            <ImagePlus className='size-4' aria-hidden='true' />
            <span className='text-[10px]'>{t('Add')}</span>
          </button>
        )}
      </div>
      <input
        accept='image/*'
        className='hidden'
        disabled={props.disabled}
        multiple
        onChange={(event) => {
          if (event.target.files) {
            acceptFiles([...event.target.files])
          }
          event.target.value = ''
        }}
        ref={inputRef}
        type='file'
      />
      <p className='text-muted-foreground text-xs'>
        {t('Optional. Up to {{count}} images, each smaller than 10 MB.', {
          count: props.max,
        })}
      </p>
    </div>
  )
}
