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
import { Download, Images, ImageUp, Loader2, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TitledCard } from '@/components/ui/titled-card'

import type { ImagePlaygroundTask } from '../types'
import { TaskCard } from './task-card'

type TaskListProps = {
  tasks: ImagePlaygroundTask[]
  isLoading: boolean
  isDeleting: boolean
  selected: ReadonlySet<string>
  isExporting: boolean
  selectableImageCount: number
  onToggleSelect: (key: string) => void
  onSelectAll: () => void
  onClearSelection: () => void
  onDownloadSelected: () => void
  onUseSelectedAsReference: () => void
  onReuse: (task: ImagePlaygroundTask) => void
  onDelete: (task: ImagePlaygroundTask) => void
}

export function TaskList(props: TaskListProps) {
  const { t } = useTranslation()
  const activeCount = props.tasks.filter(
    (task) => task.status === 'queued' || task.status === 'running'
  ).length
  const hasSelection = props.selected.size > 0

  let content: ReactNode
  if (props.isLoading) {
    content = <LoadingState />
  } else if (props.tasks.length === 0) {
    content = (
      <EmptyState
        bordered={false}
        description={t('Submitted image tasks will appear here.')}
        icon={Images}
        title={t('No image tasks yet')}
      />
    )
  } else {
    content = (
      <div className='space-y-3'>
        {props.tasks.map((task) => (
          <TaskCard
            isDeleting={props.isDeleting}
            key={task.id}
            onDelete={props.onDelete}
            onReuse={props.onReuse}
            onToggleSelect={props.onToggleSelect}
            selected={props.selected}
            task={task}
          />
        ))}
      </div>
    )
  }

  return (
    <TitledCard
      action={
        activeCount > 0 ? (
          <Badge variant='secondary'>
            {t('{{count}} in progress', { count: activeCount })}
          </Badge>
        ) : undefined
      }
      className='flex min-h-0 flex-col'
      contentClassName='min-h-0 flex-1 overflow-y-auto p-3'
      description={t(
        'Tasks keep running on the server even if you disconnect.'
      )}
      disableHoverEffect
      icon={<Images className='size-4' />}
      title={t('Task history')}
    >
      <div className='space-y-3'>
        {props.selectableImageCount > 0 && (
          <div className='border-border/70 bg-muted/30 flex flex-wrap items-center justify-between gap-2 rounded-lg border p-2'>
            <div className='flex items-center gap-2'>
              {hasSelection ? (
                <Badge variant='secondary'>
                  {t('{{count}} selected', { count: props.selected.size })}
                </Badge>
              ) : (
                <span className='text-muted-foreground text-xs'>
                  {t('Select images to download or reuse them.')}
                </span>
              )}
              <Button
                onClick={props.onSelectAll}
                size='xs'
                type='button'
                variant='ghost'
              >
                {t('Select all')}
              </Button>
              {hasSelection && (
                <Button
                  onClick={props.onClearSelection}
                  size='xs'
                  type='button'
                  variant='ghost'
                >
                  <X className='size-3.5' />
                  {t('Clear selection')}
                </Button>
              )}
            </div>
            <div className='flex items-center gap-1.5'>
              <Button
                disabled={!hasSelection || props.isExporting}
                onClick={props.onUseSelectedAsReference}
                size='xs'
                type='button'
                variant='outline'
              >
                {props.isExporting ? (
                  <Loader2 className='size-3.5 animate-spin' />
                ) : (
                  <ImageUp className='size-3.5' />
                )}
                {t('Use as reference')}
              </Button>
              <Button
                disabled={!hasSelection || props.isExporting}
                onClick={props.onDownloadSelected}
                size='xs'
                type='button'
                variant='outline'
              >
                {props.isExporting ? (
                  <Loader2 className='size-3.5 animate-spin' />
                ) : (
                  <Download className='size-3.5' />
                )}
                {t('Download selected')}
              </Button>
            </div>
          </div>
        )}
        {content}
      </div>
    </TitledCard>
  )
}
