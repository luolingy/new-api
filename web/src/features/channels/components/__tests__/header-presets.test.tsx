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
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { mergeHeaderPreset } from '../../lib/header-presets'
import { HeaderPresetButtons } from '../header-preset-buttons'

describe('header identity presets', () => {
  test('Codex selection replaces case-insensitive identities while retaining custom headers and credentials', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <HeaderPresetButtons
        value={JSON.stringify({
          '*': true,
          'user-agent': 'old',
          Originator: 'old',
          Authorization: 'Bearer {api_key}',
          'X-Custom': 'keep',
        })}
        onChange={onChange}
        disabled={false}
      />
    )
    await user.click(screen.getByRole('button', { name: 'Codex' }))
    expect(JSON.parse(onChange.mock.calls[0][0])).toEqual({
      '*': true,
      Authorization: 'Bearer {api_key}',
      'X-Custom': 'keep',
      'User-Agent': 'codex_cli_rs',
      originator: 'codex_cli_rs',
    })
  })

  test.each([
    [
      'DeepSeek Harness',
      'deepseek-harness/0.2.3-alpha.1 (+https://github.com/deepseek-ai/deepseek-harness)',
    ],
    ['OpenCode', 'opencode'],
  ])(
    '%s selection adds only an editable User-Agent identity',
    async (name, agent) => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(
        <HeaderPresetButtons value='' onChange={onChange} disabled={false} />
      )
      const button = screen.getByRole('button', { name })
      button.focus()
      await user.keyboard('{Enter}')
      expect(JSON.parse(onChange.mock.calls[0][0])).toEqual({
        'User-Agent': agent,
      })
    }
  )

  test.each(['{"broken":', '[]', 'null'])(
    'invalid object %s keeps editor input and disables presets',
    (value) => {
      const onChange = vi.fn()
      render(
        <HeaderPresetButtons
          value={value}
          onChange={onChange}
          disabled={false}
        />
      )
      for (const button of screen.getAllByRole('button')) {
        expect(button).toBeDisabled()
      }
      expect(
        screen.getByText('Fix the Request Header Override JSON first')
      ).toBeVisible()
      expect(mergeHeaderPreset(value, { 'User-Agent': 'opencode' })).toBe(value)
    }
  )

  test('switching away from Codex removes its originator but retains custom origins', () => {
    expect(
      JSON.parse(
        mergeHeaderPreset(
          '{"originator":"codex_cli_rs","User-Agent":"codex_cli_rs"}',
          { 'User-Agent': 'opencode' }
        )
      )
    ).toEqual({ 'User-Agent': 'opencode' })
    expect(
      JSON.parse(
        mergeHeaderPreset('{"originator":"custom"}', {
          'User-Agent':
            'deepseek-harness/0.2.3-alpha.1 (+https://github.com/deepseek-ai/deepseek-harness)',
        })
      )
    ).toEqual({
      originator: 'custom',
      'User-Agent':
        'deepseek-harness/0.2.3-alpha.1 (+https://github.com/deepseek-ai/deepseek-harness)',
    })
  })

  test('locked editor disables every preset', () => {
    render(<HeaderPresetButtons value='' onChange={vi.fn()} disabled />)
    for (const button of screen.getAllByRole('button')) {
      expect(button).toBeDisabled()
    }
  })
})
