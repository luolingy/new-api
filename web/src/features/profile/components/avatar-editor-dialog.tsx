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
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Slider } from '@/components/ui/slider'
import { handleServerError } from '@/lib/handle-server-error'

import { deleteUserAvatar, uploadUserAvatar } from '../api'
import {
  AVATAR_MAX_UPLOAD_BYTES,
  AVATAR_OUTPUT_SIZE,
  AVATAR_VIEWPORT_SIZE,
  clampCropOffset,
  computeCropRect,
  coverScale,
} from '../lib/avatar-crop'

type AvatarEditorDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  hasAvatar: boolean
  onSaved: (avatarUrl: string) => void
}

function drawPreview(
  canvas: HTMLCanvasElement,
  image: HTMLImageElement,
  zoom: number,
  offsetX: number,
  offsetY: number
) {
  const context = canvas.getContext('2d')
  if (!context) return
  const rect = computeCropRect(
    { width: image.naturalWidth, height: image.naturalHeight },
    { size: AVATAR_VIEWPORT_SIZE, zoom, offsetX, offsetY }
  )
  context.clearRect(0, 0, AVATAR_VIEWPORT_SIZE, AVATAR_VIEWPORT_SIZE)
  context.drawImage(image, rect.x, rect.y, rect.width, rect.height)
}

function renderAvatarBlob(
  image: HTMLImageElement,
  zoom: number,
  offsetX: number,
  offsetY: number
): Promise<Blob> {
  const rect = computeCropRect(
    { width: image.naturalWidth, height: image.naturalHeight },
    { size: AVATAR_VIEWPORT_SIZE, zoom, offsetX, offsetY }
  )
  const scale = AVATAR_OUTPUT_SIZE / AVATAR_VIEWPORT_SIZE
  const canvas = document.createElement('canvas')
  canvas.width = AVATAR_OUTPUT_SIZE
  canvas.height = AVATAR_OUTPUT_SIZE
  const context = canvas.getContext('2d')
  if (!context) {
    return Promise.reject(new Error('Canvas is unavailable'))
  }
  context.fillStyle = '#ffffff'
  context.fillRect(0, 0, AVATAR_OUTPUT_SIZE, AVATAR_OUTPUT_SIZE)
  context.drawImage(
    image,
    rect.x * scale,
    rect.y * scale,
    rect.width * scale,
    rect.height * scale
  )
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (!blob) {
          reject(new Error('Failed to encode the avatar'))
          return
        }
        resolve(blob)
      },
      'image/jpeg',
      0.9
    )
  })
}

// Placeholders replaced at call sites to keep the export helper pure.

export function AvatarEditorDialog(props: AvatarEditorDialogProps) {
  const { t } = useTranslation()
  const [image, setImage] = useState<HTMLImageElement | null>(null)
  const [zoom, setZoom] = useState(1)
  const [offset, setOffset] = useState({ x: 0, y: 0 })
  const [busy, setBusy] = useState(false)
  const [drag, setDrag] = useState<{
    pointerId: number
    startX: number
    startY: number
    originX: number
    originY: number
  } | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const objectUrlRef = useRef<string | null>(null)

  useEffect(() => {
    if (canvasRef.current && image) {
      drawPreview(canvasRef.current, image, zoom, offset.x, offset.y)
    }
  }, [image, zoom, offset])

  useEffect(() => {
    return () => {
      if (objectUrlRef.current) {
        URL.revokeObjectURL(objectUrlRef.current)
      }
    }
  }, [])

  const resetCrop = () => {
    setZoom(1)
    setOffset({ x: 0, y: 0 })
  }

  const handleFile = (file: File) => {
    if (!file.type.startsWith('image/')) {
      toast.error(t('Please choose an image file'))
      return
    }
    if (file.size > AVATAR_MAX_UPLOAD_BYTES) {
      toast.error(t('The image must be 1 MB or smaller'))
      return
    }
    if (objectUrlRef.current) {
      URL.revokeObjectURL(objectUrlRef.current)
    }
    const url = URL.createObjectURL(file)
    objectUrlRef.current = url
    const next = new Image()
    next.addEventListener('load', () => {
      setImage(next)
      resetCrop()
    })
    next.addEventListener('error', () =>
      toast.error(t('Failed to read the image'))
    )
    next.src = url
  }

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!drag || !image || drag.pointerId !== event.pointerId) return
    const scale =
      coverScale(
        { width: image.naturalWidth, height: image.naturalHeight },
        AVATAR_VIEWPORT_SIZE
      ) * zoom
    const scaledWidth = image.naturalWidth * scale
    const scaledHeight = image.naturalHeight * scale
    setOffset({
      x: clampCropOffset(
        drag.originX + (event.clientX - drag.startX),
        scaledWidth,
        AVATAR_VIEWPORT_SIZE
      ),
      y: clampCropOffset(
        drag.originY + (event.clientY - drag.startY),
        scaledHeight,
        AVATAR_VIEWPORT_SIZE
      ),
    })
  }

  const handleSave = async () => {
    if (!image) return
    setBusy(true)
    try {
      const blob = await renderAvatarBlob(image, zoom, offset.x, offset.y)
      if (blob.size > AVATAR_MAX_UPLOAD_BYTES) {
        toast.error(t('The cropped avatar is larger than 1 MB'))
        return
      }
      const result = await uploadUserAvatar(blob)
      if (!result.success) {
        toast.error(result.message || t('Failed to upload avatar'))
        return
      }
      toast.success(t('Avatar updated'))
      props.onSaved(result.data?.avatar_url ?? '')
      props.onOpenChange(false)
    } catch (error) {
      handleServerError(error, t('Failed to upload avatar'))
    } finally {
      setBusy(false)
    }
  }

  const handleRemove = async () => {
    setBusy(true)
    try {
      const result = await deleteUserAvatar()
      if (!result.success) {
        toast.error(result.message || t('Failed to remove avatar'))
        return
      }
      toast.success(t('Avatar removed'))
      props.onSaved('')
      props.onOpenChange(false)
    } catch (error) {
      handleServerError(error, t('Failed to remove avatar'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Change avatar')}</DialogTitle>
          <DialogDescription>
            {t(
              'Choose an image, drag to position it and zoom until the square crop looks right.'
            )}
          </DialogDescription>
        </DialogHeader>

        <input
          ref={fileInputRef}
          type='file'
          accept='image/png,image/jpeg'
          className='hidden'
          onChange={(event) => {
            const file = event.target.files?.[0]
            if (file) handleFile(file)
            event.target.value = ''
          }}
        />

        <div className='flex justify-center'>
          <div
            className='border-border bg-muted relative size-72 touch-none overflow-hidden rounded-xl border'
            style={{
              width: AVATAR_VIEWPORT_SIZE,
              height: AVATAR_VIEWPORT_SIZE,
            }}
            onPointerDown={(event) => {
              if (!image) return
              event.currentTarget.setPointerCapture(event.pointerId)
              setDrag({
                pointerId: event.pointerId,
                startX: event.clientX,
                startY: event.clientY,
                originX: offset.x,
                originY: offset.y,
              })
            }}
            onPointerMove={handlePointerMove}
            onPointerUp={() => setDrag(null)}
            onPointerCancel={() => setDrag(null)}
          >
            <canvas
              ref={canvasRef}
              width={AVATAR_VIEWPORT_SIZE}
              height={AVATAR_VIEWPORT_SIZE}
              className={
                image ? 'cursor-grab active:cursor-grabbing' : 'hidden'
              }
            />
            {!image && (
              <button
                type='button'
                className='text-muted-foreground flex size-full items-center justify-center text-sm'
                onClick={() => fileInputRef.current?.click()}
              >
                {t('Select an image')}
              </button>
            )}
          </div>
        </div>

        <div className='flex items-center gap-3'>
          <span className='text-muted-foreground text-xs'>{t('Zoom')}</span>
          <Slider
            min={1}
            max={3}
            step={0.01}
            value={[zoom]}
            disabled={!image || busy}
            onValueChange={(value) =>
              setZoom(Array.isArray(value) ? value[0] : value)
            }
          />
        </div>

        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={busy}
            onClick={() => fileInputRef.current?.click()}
          >
            {image ? t('Choose another image') : t('Select an image')}
          </Button>
          {props.hasAvatar && (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='text-destructive'
              disabled={busy}
              onClick={handleRemove}
            >
              {t('Remove avatar')}
            </Button>
          )}
        </div>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={busy}
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' disabled={!image || busy} onClick={handleSave}>
            {busy ? t('Saving...') : t('Save avatar')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
