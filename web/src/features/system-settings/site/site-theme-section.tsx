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
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
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
import { handleServerError } from '@/lib/handle-server-error'
import { applySiteTheme, sanitizeBackgroundImage } from '@/lib/site-theme'
import { STATUS_QUERY_KEY, STATUS_STORAGE_KEY } from '@/lib/status-query'

import { removeThemeBackground, uploadThemeBackground } from '../api'
import {
  SettingsForm,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type SiteThemeSectionProps = {
  backgroundImage: string
  glassCards: boolean
}

type SiteThemeFormValues = {
  backgroundImage: string
}

const BACKGROUND_IMAGE_OPTION = 'theme.background_image'
const GLASS_CARDS_OPTION = 'theme.glass_cards'

export function SiteThemeSection(props: SiteThemeSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const [glassCards, setGlassCards] = useState(props.glassCards)
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const form = useForm<SiteThemeFormValues>({
    resolver: zodResolver(
      z.object({
        backgroundImage: z
          .string()
          .refine(
            (value) =>
              value.trim() === '' || sanitizeBackgroundImage(value) !== '',
            {
              error: t('Must be an http(s) URL or a path starting with /'),
            }
          ),
      })
    ),
    defaultValues: { backgroundImage: props.backgroundImage },
  })

  useEffect(() => {
    form.reset({ backgroundImage: props.backgroundImage })
  }, [props.backgroundImage, form])

  useEffect(() => {
    setGlassCards(props.glassCards)
  }, [props.glassCards])

  /**
   * Persist the saved theme straight into the shared `/api/status` cache and
   * its localStorage snapshot, then apply it to `<body>`.
   *
   * Without this, the immediate `applySiteTheme` preview is fragile: any
   * `/api/status` refetch that resolves with the pre-save payload (an in-flight
   * request, a stale snapshot on reload) runs `applySiteThemeFromStatus` with
   * the old/empty `site_theme` and wipes the background — the "flashes then
   * disappears" symptom. Seeding the cache first guarantees every later status
   * apply observes the value we just saved.
   */
  const seedStatusTheme = (backgroundImage: string, glassCardsEnabled: boolean) => {
    const siteTheme = {
      background_image: backgroundImage,
      glass_cards: glassCardsEnabled,
    }
    queryClient.setQueryData<Record<string, unknown> | null>(
      STATUS_QUERY_KEY,
      (prev) => ({ ...prev, site_theme: siteTheme })
    )
    try {
      const raw = window.localStorage.getItem(STATUS_STORAGE_KEY)
      const cached = raw ? (JSON.parse(raw) as Record<string, unknown>) : {}
      window.localStorage.setItem(
        STATUS_STORAGE_KEY,
        JSON.stringify({ ...cached, site_theme: siteTheme })
      )
    } catch {
      /* Storage can be unavailable (private mode); the cache seed still holds. */
    }
  }

  const onSubmit = async (values: SiteThemeFormValues) => {
    const normalized = values.backgroundImage.trim()
    if (normalized === props.backgroundImage) return
    await updateOption.mutateAsync({
      key: BACKGROUND_IMAGE_OPTION,
      value: normalized,
    })
    // Pin the saved value before previewing so a racing stale status response
    // cannot clear it back to the previous image.
    seedStatusTheme(normalized, glassCards)
    applySiteTheme({ backgroundImage: normalized, glassCards })
  }

  const handleGlassCardsChange = (checked: boolean) => {
    const previousGlassCards = glassCards
    setGlassCards(checked)
    // The mutation hook reports failures itself; roll the switch back so the
    // control keeps matching the persisted option.
    updateOption
      .mutateAsync({ key: GLASS_CARDS_OPTION, value: checked })
      .then(() => {
        seedStatusTheme(props.backgroundImage, checked)
        applySiteTheme({
          backgroundImage: props.backgroundImage,
          glassCards: checked,
        })
      })
      .catch(() => setGlassCards(previousGlassCards))
  }

  /**
   * Refresh both the settings form and the live theme after the backend stored a
   * new background. The upload endpoint already persisted the option, so this
   * only mirrors the returned URL into the UI and the shared status cache.
   */
  const applyUploadedBackground = (url: string) => {
    form.setValue('backgroundImage', url, { shouldDirty: false })
    seedStatusTheme(url, glassCards)
    applySiteTheme({ backgroundImage: url, glassCards })
  }

  const handleBackgroundFileChange = async (
    event: React.ChangeEvent<HTMLInputElement>
  ) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setUploading(true)
    try {
      const result = await uploadThemeBackground(file)
      if (!result.success) {
        handleServerError(result, t('Failed to upload image'))
        return
      }
      const url = result.data?.background_image ?? ''
      if (url) {
        applyUploadedBackground(url)
        // Keep the settings cache fresh so re-opening the section shows the URL.
        queryClient.invalidateQueries({ queryKey: ['system-options'] })
        toast.success(t('Image uploaded'))
      }
    } catch (error) {
      handleServerError(error, t('Failed to upload image'))
    } finally {
      setUploading(false)
    }
  }

  const handleRemoveBackground = async () => {
    if (!props.backgroundImage) return
    setUploading(true)
    try {
      const result = await removeThemeBackground()
      if (!result.success) {
        handleServerError(result, t('Failed to remove image'))
        return
      }
      applyUploadedBackground('')
      queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Image removed'))
    } catch (error) {
      handleServerError(error, t('Failed to remove image'))
    } finally {
      setUploading(false)
    }
  }

  return (
    <SettingsSection title={t('Site Theme')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save site theme'
          />
          <FormField
            control={form.control}
            name='backgroundImage'
            render={({ field }) => (
              <FormItem className='lg:col-span-2'>
                <FormLabel>{t('Background image URL')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('https://example.com/background.jpg')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <div className='flex flex-wrap items-center gap-2'>
                  <input
                    ref={fileInputRef}
                    type='file'
                    accept='image/png,image/jpeg,image/webp'
                    className='hidden'
                    onChange={handleBackgroundFileChange}
                  />
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={uploading}
                    onClick={() => fileInputRef.current?.click()}
                  >
                    {uploading ? t('Uploading...') : t('Upload image')}
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    disabled={uploading || !props.backgroundImage}
                    onClick={handleRemoveBackground}
                  >
                    {t('Remove image')}
                  </Button>
                </div>
                <FormDescription>
                  {t(
                    'Upload an image file (stored on this site) or paste a URL. Leave empty to turn the site theme off.'
                  )}{' '}
                  {t(
                    'The background image is fixed and covers the viewport. Pick a low-contrast image so text stays readable.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <SettingsSwitchField
            controlId='site-theme-glass-cards'
            checked={glassCards}
            onCheckedChange={handleGlassCardsChange}
            disabled={updateOption.isPending}
            label={t('Glass card style')}
            description={t(
              'Frosted card, popover, and sidebar surfaces on top of the background image.'
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
