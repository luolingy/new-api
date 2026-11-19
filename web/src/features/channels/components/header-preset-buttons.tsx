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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { getHeaderPassthroughState } from '../lib/header-passthrough'
import { HEADER_PRESETS, mergeHeaderPreset } from '../lib/header-presets'

type HeaderPresetButtonsProps = {
  value: string | undefined
  onChange: (value: string) => void
  disabled: boolean
}

export function HeaderPresetButtons(props: HeaderPresetButtonsProps) {
  const { t } = useTranslation()
  const invalid = getHeaderPassthroughState(props.value) === 'invalid'
  return (
    <div className='space-y-2'>
      <div
        className='flex flex-wrap items-center gap-2'
        role='group'
        aria-label={t('Header presets')}
      >
        {HEADER_PRESETS.map((preset) => (
          <Button
            key={preset.name}
            type='button'
            variant='outline'
            size='sm'
            disabled={props.disabled || invalid}
            onClick={() =>
              props.onChange(mergeHeaderPreset(props.value, preset.headers))
            }
          >
            {preset.name}
          </Button>
        ))}
      </div>
      <p className='text-muted-foreground text-xs'>
        {invalid
          ? t('Fix the Request Header Override JSON first')
          : t(
              'Editable identity presets replace matching header names and keep other rules. Adjust User-Agent for your client; no version or credentials are included.'
            )}
      </p>
    </div>
  )
}
