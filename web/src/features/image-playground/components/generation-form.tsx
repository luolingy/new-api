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
import { Download, Loader2, Sparkles, Upload } from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelGroupSelector } from '@/components/model-group-selector'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  NativeSelect,
  NativeSelectOption,
  NativeSelectOptGroup,
} from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'
import type { GroupOption, ModelOption } from '@/features/playground/types'

import {
  IMAGE_PLAYGROUND_BACKGROUND_OPTIONS,
  IMAGE_PLAYGROUND_MAX_PROMPT_LENGTH,
  IMAGE_PLAYGROUND_MODERATION_OPTIONS,
  IMAGE_PLAYGROUND_OUTPUT_FORMATS,
  IMAGE_PLAYGROUND_PARTIAL_IMAGE_OPTIONS,
  IMAGE_PLAYGROUND_QUALITY_OPTIONS,
  IMAGE_PLAYGROUND_RESPONSE_FORMATS,
} from '../constants'
import {
  formatAspectRatio,
  formatImageSizeLabel,
  IMAGE_SIZE_TIERS,
  parseImageSize,
} from '../lib/size'
import type { ImagePlaygroundForm, ImagePlaygroundReference } from '../types'
import { MentionTextarea } from './mention-textarea'
import { ReferenceUploader } from './reference-uploader'

const QUALITY_LABEL_KEYS: Record<string, string> = {
  auto: 'Auto',
  low: 'Low',
  medium: 'Medium',
  high: 'High',
}

const BACKGROUND_LABEL_KEYS: Record<string, string> = {
  auto: 'Auto',
  transparent: 'Transparent',
  opaque: 'Opaque',
}

type GenerationFormProps = {
  form: ImagePlaygroundForm
  onChange: (patch: Partial<ImagePlaygroundForm>) => void
  references: ImagePlaygroundReference[]
  onReferencesChange: (references: ImagePlaygroundReference[]) => void
  models: ModelOption[]
  groups: GroupOption[]
  maxImages: number
  maxReferences: number
  disabled: boolean
  isSubmitting: boolean
  batchMode: boolean
  onBatchModeChange: (value: boolean) => void
  onExportConfig: () => void
  onImportConfig: (file: File) => void
  onSubmit: () => void
}

export function GenerationForm(props: GenerationFormProps) {
  const { t } = useTranslation()
  const importInputRef = useRef<HTMLInputElement | null>(null)
  const presetSizes = IMAGE_SIZE_TIERS.flatMap((tier) => tier.sizes)
  const customSize =
    parseImageSize(props.form.size) && !presetSizes.includes(props.form.size)
  const canSubmit =
    !props.disabled &&
    !props.isSubmitting &&
    props.form.model !== '' &&
    props.form.prompt.trim() !== ''

  return (
    <TitledCard
      className='flex min-h-0 flex-col'
      contentClassName='min-h-0 flex-1 space-y-4 overflow-y-auto p-4'
      disableHoverEffect
      icon={<Sparkles className='size-4' />}
      title={t('Generate image')}
      description={t('Runs on the server. You can close this page.')}
      action={
        <div className='flex items-center gap-1'>
          <Button
            disabled={props.disabled}
            onClick={props.onExportConfig}
            size='xs'
            title={t('Export all settings')}
            type='button'
            variant='ghost'
          >
            <Download className='size-3.5' />
            <span className='hidden sm:inline'>{t('Export')}</span>
          </Button>
          <Button
            disabled={props.disabled}
            onClick={() => importInputRef.current?.click()}
            size='xs'
            title={t('Import settings')}
            type='button'
            variant='ghost'
          >
            <Upload className='size-3.5' />
            <span className='hidden sm:inline'>{t('Import')}</span>
          </Button>
          <input
            accept='application/json,.json'
            className='hidden'
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) props.onImportConfig(file)
              event.target.value = ''
            }}
            ref={importInputRef}
            type='file'
          />
        </div>
      }
    >
      <form
        className='space-y-4'
        onSubmit={(event) => {
          event.preventDefault()
          if (canSubmit) props.onSubmit()
        }}
      >
        <div className='space-y-1.5'>
          <Label>{t('Model & group')}</Label>
          <ModelGroupSelector
            className='w-full'
            disabled={props.disabled}
            groups={props.groups}
            models={props.models}
            onGroupChange={(group) => props.onChange({ group })}
            onModelChange={(model) => props.onChange({ model })}
            selectedGroup={props.form.group}
            selectedModel={props.form.model}
          />
        </div>

        <div className='space-y-1.5'>
          <div className='flex items-center justify-between gap-2'>
            <Label htmlFor='image-playground-prompt'>{t('Prompt')}</Label>
            <label className='text-muted-foreground flex items-center gap-1.5 text-[11px]'>
              {t('Batch mode')}
              <Switch
                checked={props.batchMode}
                disabled={props.disabled}
                onCheckedChange={props.onBatchModeChange}
              />
            </label>
          </div>
          <MentionTextarea
            disabled={props.disabled}
            id='image-playground-prompt'
            maxLength={IMAGE_PLAYGROUND_MAX_PROMPT_LENGTH}
            onChange={(value) => props.onChange({ prompt: value })}
            placeholder={
              props.batchMode
                ? t(
                    'One prompt per group separated by two blank lines. Type @ to mention a reference image.'
                  )
                : t('Describe the image you want to create... Type @ to mention a reference image.')
            }
            references={props.references}
            rows={4}
            value={props.form.prompt}
          />
          <div className='text-muted-foreground flex justify-between text-[11px]'>
            <span>
              {props.references.length > 0
                ? t('Type @ to insert a reference image')
                : ''}
            </span>
            <span>
              {props.form.prompt.length}/{IMAGE_PLAYGROUND_MAX_PROMPT_LENGTH}
            </span>
          </div>
        </div>

        <div className='space-y-1.5'>
          <Label>{t('Reference images')}</Label>
          <ReferenceUploader
            disabled={props.disabled}
            max={Math.max(0, props.maxReferences)}
            onChange={props.onReferencesChange}
            references={props.references}
          />
        </div>

        <div className='grid grid-cols-2 gap-3'>
          <div className='col-span-2 space-y-1.5'>
            <Label htmlFor='image-playground-size'>{t('Image size')}</Label>
            <NativeSelect
              className='w-full'
              disabled={props.disabled}
              id='image-playground-size'
              onChange={(event) => props.onChange({ size: event.target.value })}
              value={props.form.size}
            >
              <NativeSelectOption value='auto'>{t('Auto')}</NativeSelectOption>
              {customSize && (
                <NativeSelectOption value={props.form.size}>
                  {formatImageSizeLabel(props.form.size)}
                </NativeSelectOption>
              )}
              {IMAGE_SIZE_TIERS.map((tier) => (
                <NativeSelectOptGroup key={tier.id} label={tier.id}>
                  {tier.sizes.map((size) => (
                    <NativeSelectOption key={size} value={size}>
                      {`${formatAspectRatio(size)} · ${formatImageSizeLabel(size)}`}
                    </NativeSelectOption>
                  ))}
                </NativeSelectOptGroup>
              ))}
            </NativeSelect>
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='image-playground-quality'>{t('Quality')}</Label>
            <NativeSelect
              className='w-full'
              disabled={props.disabled}
              id='image-playground-quality'
              onChange={(event) =>
                props.onChange({ quality: event.target.value })
              }
              value={props.form.quality}
            >
              {IMAGE_PLAYGROUND_QUALITY_OPTIONS.map((quality) => (
                <NativeSelectOption key={quality} value={quality}>
                  {t(QUALITY_LABEL_KEYS[quality])}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='image-playground-count'>{t('Images')}</Label>
            <Input
              className='w-full'
              disabled={props.disabled}
              id='image-playground-count'
              inputMode='numeric'
              max={Math.max(1, props.maxImages)}
              min={1}
              onChange={(event) => {
                const raw = event.target.value.replaceAll(/\D/g, '')
                const parsed = raw === '' ? 1 : Number(raw)
                const clamped = Math.min(
                  Math.max(1, parsed),
                  Math.max(1, props.maxImages)
                )
                props.onChange({ n: clamped })
              }}
              type='number'
              value={String(props.form.n)}
            />
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='image-playground-background'>
              {t('Background')}
            </Label>
            <NativeSelect
              className='w-full'
              disabled={props.disabled}
              id='image-playground-background'
              onChange={(event) =>
                props.onChange({ background: event.target.value })
              }
              value={props.form.background}
            >
              {IMAGE_PLAYGROUND_BACKGROUND_OPTIONS.map((background) => (
                <NativeSelectOption key={background} value={background}>
                  {t(BACKGROUND_LABEL_KEYS[background])}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='image-playground-moderation'>
              {t('Moderation')}
            </Label>
            <NativeSelect
              className='w-full'
              disabled={props.disabled}
              id='image-playground-moderation'
              onChange={(event) =>
                props.onChange({ moderation: event.target.value })
              }
              value={props.form.moderation}
            >
              {IMAGE_PLAYGROUND_MODERATION_OPTIONS.map((moderation) => (
                <NativeSelectOption key={moderation} value={moderation}>
                  {t(moderation === 'auto' ? 'Auto' : 'Low')}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='image-playground-format'>
              {t('Output format')}
            </Label>
            <NativeSelect
              className='w-full'
              disabled={props.disabled}
              id='image-playground-format'
              onChange={(event) =>
                props.onChange({ outputFormat: event.target.value })
              }
              value={props.form.outputFormat}
            >
              {IMAGE_PLAYGROUND_OUTPUT_FORMATS.map((format) => (
                <NativeSelectOption key={format} value={format}>
                  {format.toUpperCase()}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='image-playground-response-format'>
              {t('Response format')}
            </Label>
            <NativeSelect
              className='w-full'
              disabled={props.disabled}
              id='image-playground-response-format'
              onChange={(event) =>
                props.onChange({ responseFormat: event.target.value })
              }
              value={props.form.responseFormat}
            >
              {IMAGE_PLAYGROUND_RESPONSE_FORMATS.map((format) => (
                <NativeSelectOption key={format} value={format}>
                  {t(format === 'b64_json' ? 'Base64' : 'URL')}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>

          <div className='col-span-2 flex items-center justify-between gap-3 rounded-lg border p-2.5'>
            <div className='min-w-0 space-y-0.5'>
              <Label htmlFor='image-playground-stream'>
                {t('Stream preview')}
              </Label>
              <p className='text-muted-foreground text-[11px]'>
                {t('Show intermediate frames while the image is generated.')}
              </p>
            </div>
            <div className='flex shrink-0 items-center gap-2'>
              {props.form.stream && (
                <NativeSelect
                  aria-label={t('Preview frames')}
                  disabled={props.disabled}
                  onChange={(event) =>
                    props.onChange({
                      partialImages: Number(event.target.value),
                    })
                  }
                  value={String(props.form.partialImages)}
                >
                  {IMAGE_PLAYGROUND_PARTIAL_IMAGE_OPTIONS.map((count) => (
                    <NativeSelectOption key={count} value={String(count)}>
                      {count}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              )}
              <Switch
                checked={props.form.stream}
                disabled={props.disabled}
                id='image-playground-stream'
                onCheckedChange={(checked) =>
                  props.onChange({ stream: checked })
                }
              />
            </div>
          </div>
        </div>

        <Button className='w-full' disabled={!canSubmit} type='submit'>
          {props.isSubmitting ? (
            <Loader2 className='size-4 animate-spin' />
          ) : (
            <Sparkles className='size-4' />
          )}
          {t('Start generating')}
        </Button>
      </form>
    </TitledCard>
  )
}
