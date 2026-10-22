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
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
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
import { applySiteTheme, sanitizeBackgroundImage } from '@/lib/site-theme'

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
  const updateOption = useUpdateOption()
  const [glassCards, setGlassCards] = useState(props.glassCards)
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

  const onSubmit = async (values: SiteThemeFormValues) => {
    const normalized = values.backgroundImage.trim()
    if (normalized === props.backgroundImage) return
    await updateOption.mutateAsync({
      key: BACKGROUND_IMAGE_OPTION,
      value: normalized,
    })
    // Preview the saved theme in this tab instead of waiting for the next
    // status refresh.
    applySiteTheme({ backgroundImage: normalized, glassCards })
  }

  const handleGlassCardsChange = (checked: boolean) => {
    setGlassCards(checked)
    // The mutation hook reports failures itself; roll the switch back so the
    // control keeps matching the persisted option.
    updateOption
      .mutateAsync({ key: GLASS_CARDS_OPTION, value: checked })
      .then(() =>
        applySiteTheme({
          backgroundImage: props.backgroundImage,
          glassCards: checked,
        })
      )
      .catch(() => setGlassCards(props.glassCards))
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
                <FormDescription>
                  {t(
                    'Leave empty to turn the site theme off. Use an https:// image URL or a path served by this site (starting with /).'
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
