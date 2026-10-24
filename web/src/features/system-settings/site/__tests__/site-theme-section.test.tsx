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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { SITE_THEME_BACKGROUND_ATTRIBUTE } from '@/lib/site-theme'
import { STATUS_QUERY_KEY, STATUS_STORAGE_KEY } from '@/lib/status-query'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { SiteThemeSection } from '../site-theme-section'

function Fixture() {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <SiteThemeSection backgroundImage='' glassCards />
      </SettingsPageProvider>
    </>
  )
}

async function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Fixture }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return {
    client,
    input: await screen.findByRole('textbox', {
      name: 'Background image URL',
    }),
  }
}

beforeEach(() => {
  window.localStorage.clear()
  document.body.removeAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)
  document.body.style.removeProperty('--site-theme-background-image')
})

afterEach(() => {
  vi.restoreAllMocks()
  window.localStorage.clear()
  document.body.removeAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)
  document.body.style.removeProperty('--site-theme-background-image')
})

test('saving a background image seeds the status cache and localStorage so a stale status refresh cannot clear it', async () => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  const { client, input } = await renderSection()

  await user.type(input, 'https://cdn.example.com/bg.png')
  await user.tab()
  await user.click(screen.getByRole('button', { name: 'Save site theme' }))

  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith('/api/option/', {
      key: 'theme.background_image',
      value: 'https://cdn.example.com/bg.png',
    })
  )

  // The saved value is previewed on the body immediately.
  await waitFor(() =>
    expect(document.body.getAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)).toBe(
      'true'
    )
  )

  // ...and pinned into the shared status cache so later status applies read it.
  const cachedStatus = client.getQueryData<Record<string, unknown>>(
    STATUS_QUERY_KEY
  )
  expect(cachedStatus?.site_theme).toEqual({
    background_image: 'https://cdn.example.com/bg.png',
    glass_cards: true,
  })

  // The localStorage snapshot used on the next cold start also carries it.
  const stored = JSON.parse(
    window.localStorage.getItem(STATUS_STORAGE_KEY) ?? '{}'
  )
  expect(stored.site_theme).toEqual({
    background_image: 'https://cdn.example.com/bg.png',
    glass_cards: true,
  })
})

test('uploading a background image posts it to the local-upload endpoint and applies the returned URL', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { background_image: '/uploads/theme/1712345678-abcdef.png' },
    },
  })
  const user = userEvent.setup()
  const { client, input } = await renderSection()

  const fileInput = document.querySelector<HTMLInputElement>(
    'input[type="file"]'
  )
  expect(fileInput).not.toBeNull()
  await user.upload(
    fileInput as HTMLInputElement,
    new File(['fake-bytes'], 'bg.png', { type: 'image/png' })
  )

  await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
  const [url, body] = vi.mocked(api.post).mock.calls[0]
  expect(url).toBe('/api/option/theme/background')
  expect(body).toBeInstanceOf(FormData)

  // The returned URL is written back into the URL field...
  await waitFor(() =>
    expect(input).toHaveValue('/uploads/theme/1712345678-abcdef.png')
  )
  // ...and immediately previewed on the body.
  await waitFor(() =>
    expect(document.body.getAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)).toBe(
      'true'
    )
  )

  // The shared status cache is seeded with the uploaded URL.
  const cachedStatus = client.getQueryData<Record<string, unknown>>(
    STATUS_QUERY_KEY
  )
  expect(cachedStatus?.site_theme).toEqual({
    background_image: '/uploads/theme/1712345678-abcdef.png',
    glass_cards: true,
  })
})
