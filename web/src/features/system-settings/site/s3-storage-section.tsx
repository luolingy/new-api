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
import { useRef, useState } from 'react'
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
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { handleServerError } from '@/lib/handle-server-error'

import { testS3Connection } from '../api'
import {
  SettingsForm,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type S3StorageSectionProps = {
  storageMode: string
  localDir: string
  enabled: boolean
  endpoint: string
  publicBaseUrl: string
  region: string
  bucket: string
  pathPrefix: string
  forcePathStyle: boolean
}

type S3StorageFormValues = {
  storageMode: 'auto' | 'local' | 's3'
  localDir: string
  enabled: boolean
  endpoint: string
  publicBaseUrl: string
  region: string
  bucket: string
  pathPrefix: string
  forcePathStyle: boolean
  accessKey: string
  secretKey: string
}

const OPTION_KEYS = {
  enabled: 's3.enabled',
  endpoint: 's3.endpoint',
  publicBaseUrl: 's3.public_base_url',
  region: 's3.region',
  bucket: 's3.bucket',
  pathPrefix: 's3.path_prefix',
  forcePathStyle: 's3.force_path_style',
  accessKey: 's3.accessKey',
  secretKey: 's3.secretKey',
} as const

const STORAGE_MODE_OPTION = 'storage.mode'
const STORAGE_LOCAL_DIR_OPTION = 'storage.local_dir'

const httpUrlPattern = /^https?:\/\/[^\s]+$/i

function normalizeStorageMode(value: string): 'auto' | 'local' | 's3' {
  if (value === 'local' || value === 's3') return value
  return 'auto'
}

export function S3StorageSection(props: S3StorageSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [testing, setTesting] = useState(false)
  const baseline = useRef({
    storageMode: normalizeStorageMode(props.storageMode),
    localDir: props.localDir,
    enabled: props.enabled,
    endpoint: props.endpoint,
    publicBaseUrl: props.publicBaseUrl,
    region: props.region,
    bucket: props.bucket,
    pathPrefix: props.pathPrefix,
    forcePathStyle: props.forcePathStyle,
  })

  const form = useForm<S3StorageFormValues>({
    resolver: zodResolver(
      z.object({
        storageMode: z.enum(['auto', 'local', 's3']),
        localDir: z
          .string()
          .refine((value) => !value.includes('..'), {
            error: t('The directory must not contain ..'),
          }),
        enabled: z.boolean(),
        endpoint: z
          .string()
          .refine(
            (value) => value.trim() === '' || httpUrlPattern.test(value.trim()),
            { error: t('Must be an http(s) URL or leave it empty') }
          ),
        publicBaseUrl: z
          .string()
          .refine(
            (value) => value.trim() === '' || httpUrlPattern.test(value.trim()),
            { error: t('Must be an http(s) URL or leave it empty') }
          ),
        region: z
          .string()
          .refine((value) => /^[a-z0-9-]*$/.test(value.trim()), {
            error: t('Use lowercase letters, numbers and dashes'),
          }),
        bucket: z
          .string()
          .refine(
            (value) =>
              value.trim() === '' ||
              /^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$/.test(value.trim()),
            { error: t('Use lowercase letters, numbers, dots and dashes') }
          ),
        pathPrefix: z
          .string()
          .refine(
            (value) =>
              value.trim() === '' ||
              (/^[A-Za-z0-9._/-]+$/.test(value.trim()) &&
                !value.includes('..') &&
                !value.includes('//') &&
                !value.trim().startsWith('/') &&
                !value.trim().endsWith('/')),
            { error: t('Use a relative object prefix without .. or //') }
          ),
        forcePathStyle: z.boolean(),
        accessKey: z.string(),
        secretKey: z.string(),
      })
    ),
    defaultValues: {
      ...baseline.current,
      accessKey: '',
      secretKey: '',
    },
  })

  const onSubmit = async (values: S3StorageFormValues) => {
    const changes: { key: string; value: string | boolean }[] = []
    if (values.storageMode !== baseline.current.storageMode) {
      changes.push({ key: STORAGE_MODE_OPTION, value: values.storageMode })
    }
    if (values.localDir.trim() !== baseline.current.localDir) {
      changes.push({ key: STORAGE_LOCAL_DIR_OPTION, value: values.localDir.trim() })
    }
    for (const [field, key] of Object.entries(OPTION_KEYS)) {
      const fieldName = field as keyof S3StorageFormValues
      if (fieldName === 'accessKey' || fieldName === 'secretKey') continue
      const next = values[fieldName]
      if (
        next === baseline.current[fieldName as keyof typeof baseline.current]
      ) {
        continue
      }
      changes.push({
        key,
        value: typeof next === 'string' ? next.trim() : next,
      })
    }
    // Secrets are write-only: an empty input keeps the stored value.
    if (values.accessKey.trim() !== '') {
      changes.push({
        key: OPTION_KEYS.accessKey,
        value: values.accessKey.trim(),
      })
    }
    if (values.secretKey.trim() !== '') {
      changes.push({
        key: OPTION_KEYS.secretKey,
        value: values.secretKey.trim(),
      })
    }
    if (changes.length === 0) {
      toast.info(t('Nothing to save'))
      return
    }
    try {
      for (const change of changes) {
        await updateOption.mutateAsync(change)
      }
    } catch (error) {
      handleServerError(error, t('Failed to save object storage'))
      return
    }
    baseline.current = {
      storageMode: values.storageMode,
      localDir: values.localDir.trim(),
      enabled: values.enabled,
      endpoint: values.endpoint,
      publicBaseUrl: values.publicBaseUrl,
      region: values.region,
      bucket: values.bucket,
      pathPrefix: values.pathPrefix,
      forcePathStyle: values.forcePathStyle,
    }
    form.reset({ ...form.getValues(), accessKey: '', secretKey: '' })
  }

  const handleTestConnection = async () => {
    setTesting(true)
    try {
      const result = await testS3Connection()
      if (result.success) {
        toast.success(t('S3 connection succeeded'))
      } else {
        toast.error(result.message || t('S3 connection failed'))
      }
    } catch (error) {
      handleServerError(error, t('S3 connection failed'))
    } finally {
      setTesting(false)
    }
  }

  return (
    <SettingsSection title={t('Object Storage')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save object storage'
          />
          <FormField
            control={form.control}
            name='storageMode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Upload storage')}</FormLabel>
                <FormControl>
                  <NativeSelect
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  >
                    <NativeSelectOption value='auto'>
                      {t('Auto: S3 when configured, otherwise local disk')}
                    </NativeSelectOption>
                    <NativeSelectOption value='local'>
                      {t('Local disk')}
                    </NativeSelectOption>
                    <NativeSelectOption value='s3'>{t('S3')}</NativeSelectOption>
                  </NativeSelect>
                </FormControl>
                <FormDescription>
                  {t(
                    'Where uploaded avatars and the site background are saved.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='localDir'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Local upload directory')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='data/uploads'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Directory on the server for local uploads. Leave empty to use data/uploads. The UPLOADS_DIR environment variable overrides it.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <div className='lg:col-span-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={testing || updateOption.isPending}
              onClick={handleTestConnection}
            >
              {testing ? t('Testing...') : t('Test connection')}
            </Button>
            <p className='text-muted-foreground mt-2 text-xs'>
              {t('Save the configuration before testing the connection.')}
            </p>
          </div>
          <SettingsSwitchField
            controlId='s3-enabled'
            checked={form.watch('enabled')}
            onCheckedChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
            label={t('Enable object storage')}
            description={t('Avatars are uploaded to S3 when this is on.')}
          />
          <SettingsSwitchField
            controlId='s3-force-path-style'
            checked={form.watch('forcePathStyle')}
            onCheckedChange={(checked) =>
              form.setValue('forcePathStyle', checked, { shouldDirty: true })
            }
            label={t('Path-style requests')}
            description={t('Turn off for AWS S3 virtual-hosted buckets.')}
          />
          <FormField
            control={form.control}
            name='endpoint'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('S3 Endpoint')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='https://s3.example.com'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'API endpoint used for signing uploads, for example an R2 or MinIO address.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='publicBaseUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Public URL (optional)')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='https://cdn.example.com'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Public base URL for reading files. Leave empty to derive it from the endpoint.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='bucket'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Bucket')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='my-bucket'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='region'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Region')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='us-east-1'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='pathPrefix'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Object prefix')}</FormLabel>
                <FormControl>
                  <Input placeholder='avatars' autoComplete='off' {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'Folder inside the bucket where uploaded avatars are stored.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='accessKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Access Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    placeholder={t('Leave empty to keep the stored key')}
                    value={field.value ?? ''}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='secretKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Secret Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    placeholder={t('Leave empty to keep the stored key')}
                    value={field.value ?? ''}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
