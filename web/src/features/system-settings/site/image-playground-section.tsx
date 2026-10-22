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
import { zodResolver } from '@hookform/resolvers/zod'
import { useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type ImagePlaygroundSectionProps = {
  enabled: boolean
  models: string
  maxReferenceImages: number
  maxImagesPerTask: number
  retentionDays: number
}

type ImagePlaygroundFormValues = {
  enabled: boolean
  models: string
  maxReferenceImages: number
  maxImagesPerTask: number
  retentionDays: number
}

const OPTION_KEYS = {
  enabled: 'image_playground.enabled',
  models: 'image_playground.models',
  maxReferenceImages: 'image_playground.max_reference_images',
  maxImagesPerTask: 'image_playground.max_images_per_task',
  retentionDays: 'image_playground.retention_days',
} as const

export function ImagePlaygroundSection(props: ImagePlaygroundSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baseline = useRef<ImagePlaygroundFormValues>({
    enabled: props.enabled,
    models: props.models,
    maxReferenceImages: props.maxReferenceImages,
    maxImagesPerTask: props.maxImagesPerTask,
    retentionDays: props.retentionDays,
  })

  const form = useForm<ImagePlaygroundFormValues>({
    resolver: zodResolver(
      z.object({
        enabled: z.boolean(),
        models: z
          .string()
          .refine(
            (value) =>
              value
                .split(',')
                .every(
                  (entry) => entry.trim() === '' || !/\s/.test(entry.trim())
                ),
            { error: t('Model names must not contain spaces') }
          ),
        maxReferenceImages: z.number().int().min(0).max(16),
        maxImagesPerTask: z.number().int().min(1).max(8),
        retentionDays: z.number().int().min(0).max(90),
      })
    ),
    defaultValues: { ...baseline.current },
  })

  async function onSubmit(values: ImagePlaygroundFormValues) {
    const changes: { key: string; value: string | boolean | number }[] = []
    for (const [field, key] of Object.entries(OPTION_KEYS)) {
      const fieldName = field as keyof ImagePlaygroundFormValues
      const next = values[fieldName]
      const previous = baseline.current[fieldName]
      if (next === previous) continue
      changes.push({
        key,
        value: typeof next === 'string' ? next.trim() : next,
      })
    }
    if (changes.length === 0) {
      toast.info(t('Nothing to save'))
      return
    }
    for (const change of changes) {
      await updateOption.mutateAsync(change)
    }
    baseline.current = { ...values }
  }

  return (
    <SettingsSection title={t('Image Playground')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            isSaving={updateOption.isPending}
            onSave={form.handleSubmit(onSubmit)}
            saveLabel='Save image playground settings'
          />
          <div className='lg:col-span-2'>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Image tasks run on the server and keep running after the client disconnects. Images are stored on this server instance.'
              )}
            </p>
          </div>
          <SettingsSwitchField
            checked={form.watch('enabled')}
            controlId='image-playground-enabled'
            description={t(
              'When disabled, no user can submit new image generation tasks.'
            )}
            label={t('Enable image playground')}
            onCheckedChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
          />
          <FormField
            control={form.control}
            name='models'
            render={({ field }) => (
              <FormItem className='lg:col-span-2'>
                <FormLabel>{t('Allowed models')}</FormLabel>
                <FormControl>
                  <Textarea
                    placeholder='gpt-image-1, dall-e-3'
                    rows={2}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Comma separated model names. Leave empty to allow every model the user can access.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='maxReferenceImages'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Max reference images')}</FormLabel>
                <FormControl>
                  <Input
                    max={16}
                    min={0}
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                    type='number'
                    value={field.value}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='maxImagesPerTask'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Max images per task')}</FormLabel>
                <FormControl>
                  <Input
                    max={8}
                    min={1}
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                    type='number'
                    value={field.value}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='retentionDays'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Keep finished tasks for (days)')}</FormLabel>
                <FormControl>
                  <Input
                    max={90}
                    min={0}
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                    type='number'
                    value={field.value}
                  />
                </FormControl>
                <FormDescription>
                  {t('Use 0 to keep tasks until the user deletes them.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
