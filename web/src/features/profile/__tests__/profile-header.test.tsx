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
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ProfileHeader } from '../components/profile-header'
import type { UserProfile } from '../types'

// jsdom never fires image load events, so the Base UI avatar keeps the image
// unmounted. Rendering a plain img keeps the wiring assertion meaningful.
vi.mock('@/components/ui/avatar', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/avatar')>()
  return {
    ...actual,
    AvatarImage: (props: { src?: string; alt?: string }) =>
      props.src ? <img src={props.src} alt={props.alt} /> : null,
  }
})

const profile: UserProfile = {
  id: 1,
  username: 'alice',
  display_name: 'Alice',
  role: 1,
  group: 'default',
  quota: 1000000,
  used_quota: 0,
  request_count: 0,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 0,
}

describe('ProfileHeader avatar', () => {
  it('renders the stored avatar as an image named after the user', () => {
    render(
      <ProfileHeader
        profile={{
          ...profile,
          avatar_url: 'https://cdn.example.com/avatars/1/a.png',
        }}
        loading={false}
      />
    )

    expect(screen.getByAltText('Alice')).toHaveAttribute(
      'src',
      'https://cdn.example.com/avatars/1/a.png'
    )
  })

  it('shows the username initial when no avatar is stored', () => {
    render(<ProfileHeader profile={profile} loading={false} />)

    expect(screen.queryByAltText('Alice')).toBeNull()
    expect(screen.getByText('A')).toBeInTheDocument()
  })

  it('offers an accessible action to change the avatar', () => {
    render(<ProfileHeader profile={profile} loading={false} />)

    expect(
      screen.getByRole('button', { name: 'Change avatar' })
    ).toBeInTheDocument()
  })
})
