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
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Download,
  Loader2,
  RotateCcw,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import dayjs from '@/lib/dayjs'

import { imagePlaygroundAssetUrl, imagePlaygroundPartialUrl } from '../api'
import { imagePlaygroundImageKey } from '../lib/selection'
import { formatImageSizeLabel } from '../lib/size'
import type { ImagePlaygroundStatus, ImagePlaygroundTask } from '../types'

type TaskCardProps = {
  task: ImagePlaygroundTask
  isDeleting: boolean
  selected: ReadonlySet<string>
  onToggleSelect: (key: string) => void
  onReuse: (task: ImagePlaygroundTask) => void
  onDelete: (task: ImagePlaygroundTask) => void
}

const STATUS_BADGE_VARIANT: Record<
  ImagePlaygroundStatus,
  'secondary' | 'default' | 'outline' | 'destructive'
> = {
  queued: 'outline',
  running: 'default',
  succeeded: 'secondary',
  failed: 'destructive',
}

const STATUS_LABEL_KEYS: Record<ImagePlaygroundStatus, string> = {
  queued: 'Queued',
  running: 'Generating',
  succeeded: 'Completed',
  failed: 'Failed',
}

export function TaskCard(props: TaskCardProps) {
  const { t } = useTranslation()
  const [previewIndex, setPreviewIndex] = useState<number | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const preview =
    previewIndex === null ? null : (props.task.images[previewIndex] ?? null)
  const isActive =
    props.task.status === 'queued' || props.task.status === 'running'
  const imageCount = props.task.images.length

  function showAdjacentImage(step: number) {
    setPreviewIndex((current) => {
      if (current === null || imageCount === 0) return current
      return (current + step + imageCount) % imageCount
    })
  }

  return (
    <div className='border-border/70 bg-card/60 space-y-2.5 rounded-xl border p-3'>
      <div className='flex items-start justify-between gap-2'>
        <div className='min-w-0 flex-1 space-y-1'>
          <p className='line-clamp-3 text-sm break-words'>
            {props.task.prompt}
          </p>
          <div className='text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px]'>
            <span className='bg-muted text-foreground/80 max-w-[14rem] truncate rounded px-1.5 py-0.5 font-medium'>
              {props.task.model}
            </span>
            {props.task.group !== '' && (
              <span>{t('Group: {{group}}', { group: props.task.group })}</span>
            )}
            <span>{formatImageSizeLabel(props.task.size)}</span>
            <span>×{props.task.n}</span>
            {props.task.stream && <span>{t('Streamed')}</span>}
            <span>
              {dayjs.unix(props.task.created_at).format('MM-DD HH:mm')}
            </span>
          </div>
        </div>
        <Badge
          className='shrink-0'
          variant={STATUS_BADGE_VARIANT[props.task.status]}
        >
          {isActive && <Loader2 className='size-3 animate-spin' />}
          {t(STATUS_LABEL_KEYS[props.task.status])}
        </Badge>
      </div>

      {props.task.error !== '' && (
        <p className='text-destructive text-xs break-words'>
          {props.task.error}
        </p>
      )}

      {props.task.partial && imageCount === 0 && (
        <div className='border-border/70 relative overflow-hidden rounded-lg border'>
          <img
            alt={t('Preview frame')}
            className='max-h-64 w-full object-contain'
            src={`${imagePlaygroundPartialUrl(props.task.id)}?v=${props.task.partial.updated_at}`}
          />
          <span className='bg-background/85 text-foreground/80 absolute top-2 left-2 inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px]'>
            <Loader2 className='size-3 animate-spin' />
            {t('Preview frame {{index}}', {
              index: props.task.partial.index + 1,
            })}
          </span>
        </div>
      )}

      {imageCount > 0 && (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
          {props.task.images.map((image, index) => {
            const key = imagePlaygroundImageKey(props.task.id, index)
            const isSelected = props.selected.has(key)
            return (
              <div
                key={image.index}
                className='border-border/70 bg-muted/30 group relative aspect-square overflow-hidden rounded-lg border'
              >
                <button
                  aria-label={t('Open image {{index}}', { index: index + 1 })}
                  className='size-full'
                  onClick={() => setPreviewIndex(index)}
                  type='button'
                >
                  <img
                    alt={t('Generated image {{index}}', { index: index + 1 })}
                    className='size-full object-cover transition-transform duration-200 group-hover:scale-105'
                    loading='lazy'
                    src={image.url}
                  />
                </button>
                <button
                  aria-checked={isSelected}
                  aria-label={t('Select image {{index}}', { index: index + 1 })}
                  className='bg-background/85 hover:bg-background absolute top-1.5 right-1.5 inline-flex size-5 items-center justify-center rounded border'
                  onClick={() => props.onToggleSelect(key)}
                  role='checkbox'
                  type='button'
                >
                  {isSelected && <Check className='text-primary size-3.5' />}
                </button>
              </div>
            )
          })}
        </div>
      )}

      <div className='flex items-center justify-end gap-1.5'>
        <Button
          onClick={() => props.onReuse(props.task)}
          size='xs'
          type='button'
          variant='ghost'
        >
          <RotateCcw className='size-3.5' />
          {t('Reuse')}
        </Button>
        <Button
          onClick={() => setConfirmOpen(true)}
          size='xs'
          type='button'
          variant='ghost'
        >
          <Trash2 className='size-3.5' />
          {t('Delete')}
        </Button>
      </div>

      <Dialog
        open={preview !== null}
        onOpenChange={(open) => {
          if (!open) setPreviewIndex(null)
        }}
      >
        <DialogContent className='max-w-4xl'>
          <DialogHeader>
            <DialogTitle>
              {t('Image {{index}} of {{total}}', {
                index: (previewIndex ?? 0) + 1,
                total: imageCount,
              })}
            </DialogTitle>
            <DialogDescription>
              {preview?.revised_prompt || props.task.prompt}
            </DialogDescription>
          </DialogHeader>
          {preview && (
            <div
              className='space-y-3'
              onKeyDown={(event) => {
                if (event.key === 'ArrowLeft') {
                  event.preventDefault()
                  showAdjacentImage(-1)
                }
                if (event.key === 'ArrowRight') {
                  event.preventDefault()
                  showAdjacentImage(1)
                }
              }}
              tabIndex={-1}
            >
              <div className='bg-muted/30 flex items-center justify-center overflow-auto rounded-lg'>
                <img
                  alt={t('Generated image')}
                  className='max-h-[70vh] w-auto cursor-zoom-in object-contain transition-transform duration-200 hover:scale-150'
                  src={preview.url}
                />
              </div>
              <div className='flex items-center justify-between gap-2'>
                <div className='flex items-center gap-1'>
                  <Button
                    aria-label={t('Previous image')}
                    disabled={imageCount < 2}
                    onClick={() => showAdjacentImage(-1)}
                    size='icon-sm'
                    type='button'
                    variant='outline'
                  >
                    <ChevronLeft className='size-4' />
                  </Button>
                  <Button
                    aria-label={t('Next image')}
                    disabled={imageCount < 2}
                    onClick={() => showAdjacentImage(1)}
                    size='icon-sm'
                    type='button'
                    variant='outline'
                  >
                    <ChevronRight className='size-4' />
                  </Button>
                </div>
                <div className='flex items-center gap-2'>
                  <Button
                    onClick={() =>
                      props.onToggleSelect(
                        imagePlaygroundImageKey(props.task.id, preview.index)
                      )
                    }
                    size='sm'
                    type='button'
                    variant='outline'
                  >
                    <Check className='size-4' />
                    {props.selected.has(
                      imagePlaygroundImageKey(props.task.id, preview.index)
                    )
                      ? t('Unselect')
                      : t('Select')}
                  </Button>
                  <Button
                    render={
                      <a
                        download={`image-playground-${props.task.id}-${preview.index}`}
                        href={imagePlaygroundAssetUrl(
                          props.task.id,
                          preview.index
                        )}
                      />
                    }
                    size='sm'
                    variant='outline'
                  >
                    <Download className='size-4' />
                    {t('Download')}
                  </Button>
                </div>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        confirmText={t('Delete')}
        desc={t('Delete this image task and its generated images?')}
        destructive
        handleConfirm={() => {
          setConfirmOpen(false)
          props.onDelete(props.task)
        }}
        isLoading={props.isDeleting}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title={t('Delete image task')}
      />
    </div>
  )
}
