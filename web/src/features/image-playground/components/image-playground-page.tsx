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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Info } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

import {
  createImagePlaygroundTask,
  deleteImagePlaygroundTask,
  fetchImagePlaygroundBlob,
  getImagePlaygroundConfig,
  imagePlaygroundAssetUrl,
  listImagePlaygroundTasks,
} from '../api'
import {
  IMAGE_PLAYGROUND_MAX_REFERENCE_BYTES,
  IMAGE_PLAYGROUND_POLL_INTERVAL_MS,
  DEFAULT_IMAGE_PLAYGROUND_FORM,
} from '../constants'
import { useImagePlaygroundOptions } from '../hooks/use-image-playground-options'
import {
  buildImagePlaygroundFormData,
  readStoredImagePlaygroundForm,
  storeImagePlaygroundForm,
} from '../lib/form'
import {
  imagePlaygroundImageKey,
  parseImagePlaygroundImageKey,
} from '../lib/selection'
import type {
  ImagePlaygroundForm,
  ImagePlaygroundReference,
  ImagePlaygroundTask,
} from '../types'
import { GenerationForm } from './generation-form'
import { TaskList } from './task-list'

const TASKS_QUERY_KEY = ['image-playground-tasks'] as const

export function ImagePlayground() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<ImagePlaygroundForm>(
    readStoredImagePlaygroundForm
  )
  const [references, setReferences] = useState<ImagePlaygroundReference[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [isExporting, setIsExporting] = useState(false)
  const referencesRef = useRef(references)
  referencesRef.current = references

  const configQuery = useQuery({
    queryKey: ['image-playground-config'],
    queryFn: getImagePlaygroundConfig,
  })
  const config = configQuery.data
  const options = useImagePlaygroundOptions({
    group: form.group,
    allowedModels: config?.allowed_models ?? [],
  })

  const tasksQuery = useQuery({
    queryKey: TASKS_QUERY_KEY,
    queryFn: listImagePlaygroundTasks,
    refetchInterval: (query) => {
      const tasks = query.state.data
      if (!tasks) return false
      const hasActive = tasks.some(
        (task) => task.status === 'queued' || task.status === 'running'
      )
      return hasActive ? IMAGE_PLAYGROUND_POLL_INTERVAL_MS : false
    },
  })

  const submitMutation = useMutation({
    mutationFn: createImagePlaygroundTask,
    onSuccess: () => {
      toast.success(
        t('Task submitted. It keeps generating even if you close the page.')
      )
      void queryClient.invalidateQueries({ queryKey: TASKS_QUERY_KEY })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (taskId: string) => deleteImagePlaygroundTask(taskId),
    onSuccess: (_data, taskId) => {
      toast.success(t('Image task deleted'))
      const prefix = `${taskId}:`
      setSelected((current) => {
        const next = new Set(
          [...current].filter((key) => !key.startsWith(prefix))
        )
        return next.size === current.size ? current : next
      })
      void queryClient.invalidateQueries({ queryKey: TASKS_QUERY_KEY })
    },
  })

  useEffect(() => {
    storeImagePlaygroundForm(form)
  }, [form])

  useEffect(() => {
    if (form.group !== '' || options.groups.length === 0) return
    setForm((current) => ({ ...current, group: options.groups[0].value }))
  }, [form.group, options.groups])

  useEffect(() => {
    if (options.models.length === 0) return
    const hasCurrentModel = options.models.some(
      (model) => model.value === form.model
    )
    if (hasCurrentModel) return
    setForm((current) => ({ ...current, model: options.models[0].value }))
  }, [form.model, options.models])

  useEffect(
    () => () => {
      for (const reference of referencesRef.current) {
        URL.revokeObjectURL(reference.previewUrl)
      }
    },
    []
  )

  const tasks = tasksQuery.data ?? []
  const selectableImageCount = tasks.reduce(
    (total, task) => total + task.images.length,
    0
  )

  function updateForm(patch: Partial<ImagePlaygroundForm>) {
    setForm((current) => {
      const next = { ...current, ...patch }
      if (patch.group !== undefined && patch.group !== current.group) {
        next.model = ''
      }
      return next
    })
  }

  function handleSubmit() {
    if (!form.model) {
      toast.error(t('Please select a model'))
      return
    }
    if (!form.prompt.trim()) {
      toast.error(t('Please enter a prompt'))
      return
    }
    submitMutation.mutate(
      buildImagePlaygroundFormData(
        form,
        references.map((reference) => reference.file)
      )
    )
  }

  function handleReuse(task: ImagePlaygroundTask) {
    setForm((current) => ({
      ...current,
      model: task.model,
      group: task.group || current.group,
      prompt: task.prompt,
      size: task.size || DEFAULT_IMAGE_PLAYGROUND_FORM.size,
      quality: task.quality || DEFAULT_IMAGE_PLAYGROUND_FORM.quality,
      background: task.background || DEFAULT_IMAGE_PLAYGROUND_FORM.background,
      moderation: task.moderation || DEFAULT_IMAGE_PLAYGROUND_FORM.moderation,
      outputFormat:
        task.output_format || DEFAULT_IMAGE_PLAYGROUND_FORM.outputFormat,
      responseFormat:
        task.response_format || DEFAULT_IMAGE_PLAYGROUND_FORM.responseFormat,
      stream: task.stream === true,
      partialImages:
        task.partial_images > 0
          ? task.partial_images
          : DEFAULT_IMAGE_PLAYGROUND_FORM.partialImages,
      n: task.n || 1,
    }))
    toast.success(t('Task settings restored'))
  }

  function handleDelete(task: ImagePlaygroundTask) {
    deleteMutation.mutate(task.id)
  }

  function toggleSelected(key: string) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  function selectAllImages() {
    const next = new Set<string>()
    for (const task of tasks) {
      for (let index = 0; index < task.images.length; index++) {
        next.add(imagePlaygroundImageKey(task.id, index))
      }
    }
    setSelected(next)
  }

  function resolveSelectedImages() {
    const resolved: { task: ImagePlaygroundTask; index: number }[] = []
    for (const key of selected) {
      const parsed = parseImagePlaygroundImageKey(key)
      if (parsed === null) continue
      const task = tasks.find((item) => item.id === parsed.taskId)
      if (task === undefined || task.images[parsed.index] === undefined) {
        continue
      }
      resolved.push({ task, index: parsed.index })
    }
    return resolved
  }

  function imageFileName(task: ImagePlaygroundTask, index: number) {
    const mimeType = task.images[index]?.mime_type ?? 'image/png'
    return `image-playground-${task.id}-${index}.${mimeType.split('/')[1] || 'png'}`
  }

  async function downloadSelected() {
    const resolved = resolveSelectedImages()
    if (resolved.length === 0) return
    setIsExporting(true)
    try {
      for (const item of resolved) {
        const blob = await fetchImagePlaygroundBlob(
          imagePlaygroundAssetUrl(item.task.id, item.index)
        )
        const url = URL.createObjectURL(blob)
        const link = document.createElement('a')
        link.href = url
        link.download = imageFileName(item.task, item.index)
        document.body.append(link)
        link.click()
        link.remove()
        URL.revokeObjectURL(url)
        // 浏览器会丢弃间隔过短的连续下载，这里留出一点间隔
        await new Promise((resolve) => setTimeout(resolve, 300))
      }
    } catch {
      toast.error(t('Failed to download the selected images'))
    } finally {
      setIsExporting(false)
    }
  }

  async function addSelectedAsReference() {
    const resolved = resolveSelectedImages()
    if (resolved.length === 0) return
    const maxReferences = config?.max_reference_images ?? 4
    const available = maxReferences - references.length
    if (available <= 0) {
      toast.error(
        t('You can attach at most {{count}} reference images', {
          count: maxReferences,
        })
      )
      return
    }
    setIsExporting(true)
    try {
      const added: ImagePlaygroundReference[] = []
      for (const item of resolved.slice(0, available)) {
        const blob = await fetchImagePlaygroundBlob(
          imagePlaygroundAssetUrl(item.task.id, item.index)
        )
        if (blob.size > IMAGE_PLAYGROUND_MAX_REFERENCE_BYTES) continue
        const file = new File([blob], imageFileName(item.task, item.index), {
          type: item.task.images[item.index].mime_type || blob.type,
        })
        added.push({
          id: crypto.randomUUID(),
          file,
          previewUrl: URL.createObjectURL(file),
        })
      }
      if (added.length === 0) {
        toast.error(t('The selected images could not be used as references'))
        return
      }
      setReferences((current) => [...current, ...added])
      toast.success(
        t('Added {{count}} images as references', { count: added.length })
      )
    } catch {
      toast.error(t('Failed to load the selected images'))
    } finally {
      setIsExporting(false)
    }
  }

  const unavailable = config !== undefined && !config.enabled
  const hasModelOptions = options.models.length > 0

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Image Playground')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {configQuery.isLoading ? (
          <LoadingState />
        ) : (
          <div className='flex h-full min-h-0 flex-col gap-3'>
            {unavailable && (
              <Alert variant='destructive'>
                <Info className='size-4' />
                <AlertTitle>{t('Image generation is unavailable')}</AlertTitle>
                <AlertDescription>{config?.message}</AlertDescription>
              </Alert>
            )}
            {!unavailable &&
              hasModelOptions === false &&
              !options.isLoading && (
                <Alert>
                  <Info className='size-4' />
                  <AlertTitle>{t('No image models available')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'No model is available for the current group. Ask an administrator to check the channel configuration.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
            <div className='grid min-h-0 flex-1 gap-3 overflow-y-auto lg:grid-cols-[minmax(0,24rem)_minmax(0,1fr)] lg:overflow-hidden'>
              <GenerationForm
                disabled={unavailable}
                form={form}
                groups={options.groups}
                isSubmitting={submitMutation.isPending}
                maxImages={config?.max_images_per_task ?? 4}
                maxReferences={config?.max_reference_images ?? 4}
                models={options.models}
                onChange={updateForm}
                onReferencesChange={setReferences}
                onSubmit={handleSubmit}
                references={references}
              />
              <TaskList
                isDeleting={deleteMutation.isPending}
                isExporting={isExporting}
                isLoading={tasksQuery.isLoading}
                onClearSelection={() => setSelected(new Set())}
                onDelete={handleDelete}
                onDownloadSelected={() => void downloadSelected()}
                onReuse={handleReuse}
                onSelectAll={selectAllImages}
                onToggleSelect={toggleSelected}
                onUseSelectedAsReference={() => void addSelectedAsReference()}
                selectableImageCount={selectableImageCount}
                selected={selected}
                tasks={tasks}
              />
            </div>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
