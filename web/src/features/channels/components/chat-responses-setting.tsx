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
import { useId } from 'react'
import { type UseFormReturn, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

import type { ChannelFormValues } from '../lib/channel-form'

type ChatResponsesSettingProps = {
  form: UseFormReturn<ChannelFormValues>
  disabled: boolean
  inline: boolean
}

export function ChatResponsesSetting(props: ChatResponsesSettingProps) {
  const { t } = useTranslation()
  const id = useId()
  const enabled = useWatch({
    control: props.form.control,
    name: 'chat_completions_to_responses_enabled',
  })
  let value = 'inherit'
  if (enabled === true) value = 'enabled'
  if (enabled === false) value = 'disabled'

  return (
    <div
      className={
        props.inline ? 'flex flex-wrap items-center gap-2' : 'space-y-2 py-3'
      }
    >
      <Label htmlFor={id}>{t('Chat Completions to Responses')}</Label>
      <NativeSelect
        id={id}
        value={value}
        disabled={props.disabled}
        aria-describedby={props.inline ? undefined : `${id}-description`}
        onChange={(event) => {
          let next: boolean | undefined
          if (event.target.value === 'enabled') next = true
          if (event.target.value === 'disabled') next = false
          props.form.setValue('chat_completions_to_responses_enabled', next, {
            shouldDirty: true,
            shouldValidate: true,
          })
        }}
      >
        <NativeSelectOption value='inherit'>
          {t('Inherit global default')}
        </NativeSelectOption>
        <NativeSelectOption value='enabled'>{t('Enabled')}</NativeSelectOption>
        <NativeSelectOption value='disabled'>
          {t('Disabled')}
        </NativeSelectOption>
      </NativeSelect>
      {!props.inline && (
        <p id={`${id}-description`} className='text-muted-foreground text-xs'>
          {t(
            'Inherit the global policy, enable conversion regardless of its model filter, or disable conversion. Requires upstream Responses support. Body passthrough bypasses conversion.'
          )}
        </p>
      )}
    </div>
  )
}
