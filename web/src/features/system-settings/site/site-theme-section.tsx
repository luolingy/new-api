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
import { Slider } from '@/components/ui/slider'
import { handleServerError } from '@/lib/handle-server-error'
import {
  applySiteTheme,
  normalizeBackgroundPosition,
  normalizeCardOpacity,
  sanitizeBackgroundImage,
} from '@/lib/site-theme'
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
  backgroundPosition: string
  glassCards: boolean
  cardOpacity: number
}

type SiteThemeFormValues = {
  backgroundImage: string
}

const BACKGROUND_IMAGE_OPTION = 'theme.background_image'
const BACKGROUND_POSITION_OPTION = 'theme.background_position'
const GLASS_CARDS_OPTION = 'theme.glass_cards'
const CARD_OPACITY_OPTION = 'theme.card_opacity'

/** Focus presets in the order they render in the 3x3 picker. */
const POSITION_GRID: string[][] = [
  ['left top', 'center top', 'right top'],
  ['left center', 'center', 'right center'],
  ['left bottom', 'center bottom', 'right bottom'],
]

/** English source strings used as i18n keys for the focus picker buttons. */
const POSITION_LABEL_KEYS: Record<string, string> = {
  'left top': 'Top left',
  'center top': 'Top center',
  'right top': 'Top right',
  'left center': 'Left center',
  center: 'Center',
  'right center': 'Right center',
  'left bottom': 'Bottom left',
  'center bottom': 'Bottom center',
  'right bottom': 'Bottom right',
}

export function SiteThemeSection(props: SiteThemeSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const [glassCards, setGlassCards] = useState(props.glassCards)
  const [backgroundPosition, setBackgroundPosition] = useState(
    normalizeBackgroundPosition(props.backgroundPosition)
  )
  const [cardOpacity, setCardOpacity] = useState(
    normalizeCardOpacity(props.cardOpacity)
  )
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

  useEffect(() => {
    setBackgroundPosition(normalizeBackgroundPosition(props.backgroundPosition))
  }, [props.backgroundPosition])

  useEffect(() => {
    setCardOpacity(normalizeCardOpacity(props.cardOpacity))
  }, [props.cardOpacity])

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
  const seedStatusTheme = (
    backgroundImage: string,
    backgroundPos: string,
    glassCardsEnabled: boolean,
    opacity: number
  ) => {
    const siteTheme = {
      background_image: backgroundImage,
      background_position: backgroundPos,
      glass_cards: glassCardsEnabled,
      card_opacity: opacity,
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

  const previewTheme = (
    image: string,
    position: string,
    glass: boolean,
    opacity: number
  ) => {
    seedStatusTheme(image, position, glass, opacity)
    applySiteTheme({
      backgroundImage: image,
      backgroundPosition: position,
      glassCards: glass,
      cardOpacity: opacity,
    })
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
    previewTheme(normalized, backgroundPosition, glassCards, cardOpacity)
  }

  const handleGlassCardsChange = (checked: boolean) => {
    const previousGlassCards = glassCards
    setGlassCards(checked)
    // The mutation hook reports failures itself; roll the switch back so the
    // control keeps matching the persisted option.
    updateOption
      .mutateAsync({ key: GLASS_CARDS_OPTION, value: checked })
      .then(() =>
        previewTheme(props.backgroundImage, backgroundPosition, checked, cardOpacity)
      )
      .catch(() => setGlassCards(previousGlassCards))
  }

  const handleBackgroundPositionChange = (next: string) => {
    const normalized = normalizeBackgroundPosition(next)
    if (normalized === backgroundPosition) return
    const previous = backgroundPosition
    setBackgroundPosition(normalized)
    updateOption
      .mutateAsync({ key: BACKGROUND_POSITION_OPTION, value: normalized })
      .then(() =>
        previewTheme(props.backgroundImage, normalized, glassCards, cardOpacity)
      )
      .catch(() => setBackgroundPosition(previous))
  }

  /**
   * Card opacity is previewed instantly (so dragging the slider is live) but
   * only persisted when the user releases the slider, to avoid a request per
   * pixel. `onValueCommit` calls this.
   */
  const previewCardOpacity = (next: number) => {
    const normalized = normalizeCardOpacity(next)
    setCardOpacity(normalized)
    previewTheme(
      props.backgroundImage,
      backgroundPosition,
      glassCards,
      normalized
    )
  }

  const commitCardOpacity = (next: number) => {
    const normalized = normalizeCardOpacity(next)
    if (normalized === normalizeCardOpacity(props.cardOpacity)) return
    updateOption
      .mutateAsync({ key: CARD_OPACITY_OPTION, value: normalized })
      .catch(() => setCardOpacity(normalizeCardOpacity(props.cardOpacity)))
  }

  /**
   * Refresh both the settings form and the live theme after the backend stored a
   * new background. The upload endpoint already persisted the option, so this
   * only mirrors the returned URL into the UI and the shared status cache.
   */
  const applyUploadedBackground = (url: string) => {
    form.setValue('backgroundImage', url, { shouldDirty: false })
    previewTheme(url, backgroundPosition, glassCards, cardOpacity)
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
          <div className='space-y-2 lg:col-span-2'>
            <div className='text-sm font-medium leading-none'>
              {t('Background focus point')}
            </div>
            <div className='flex flex-wrap items-center gap-3'>
              <div
                className='grid grid-cols-3 gap-1 rounded-lg border p-1'
                role='group'
                aria-label={t('Background focus point')}
              >
                {POSITION_GRID.flat().map((position) => {
                  const selected = position === backgroundPosition
                  return (
                    <button
                      key={position}
                      type='button'
                      aria-label={t(POSITION_LABEL_KEYS[position])}
                      aria-pressed={selected}
                      disabled={updateOption.isPending || !props.backgroundImage}
                      onClick={() => handleBackgroundPositionChange(position)}
                      className={`flex size-9 items-center justify-center rounded-md border transition-colors ${
                        selected
                          ? 'border-primary bg-primary/10'
                          : 'border-border/60 hover:bg-muted'
                      }`}
                    >
                      <span
                        className={`size-2 rounded-full ${
                          selected ? 'bg-primary' : 'bg-muted-foreground/50'
                        }`}
                      />
                    </button>
                  )
                })}
              </div>
              <p className='text-muted-foreground max-w-xs text-xs'>
                {t(
                  'Which part of the image stays visible when the screen crops it. Pick the side that matters for a landscape image on a portrait screen.'
                )}
              </p>
            </div>
          </div>
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
          <div className='space-y-2 lg:col-span-2'>
            <div className='flex items-center justify-between'>
              <label
                htmlFor='site-theme-card-opacity'
                className='text-sm font-medium leading-none'
              >
                {t('Card opacity')}
              </label>
              <span className='text-muted-foreground text-sm tabular-nums'>
                {cardOpacity}%
              </span>
            </div>
            <Slider
              id='site-theme-card-opacity'
              min={0}
              max={100}
              step={1}
              value={[cardOpacity]}
              disabled={
                updateOption.isPending ||
                !props.backgroundImage ||
                !glassCards
              }
              onValueChange={(value) => {
                const next = Array.isArray(value) ? value[0] : value
                previewCardOpacity(next)
              }}
              onValueCommitted={(value) => {
                const next = Array.isArray(value) ? value[0] : value
                commitCardOpacity(next)
              }}
            />
            <p className='text-muted-foreground text-xs'>
              {t(
                'Opacity of the glass panels (cards, popovers, sidebar). Lower values let more of the background image show through.'
              )}
            </p>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
