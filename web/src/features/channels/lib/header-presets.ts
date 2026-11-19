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
import { getHeaderPassthroughState } from './header-passthrough'

// Editable identity templates, without credentials or version claims.
// Codex originator is also used by relay/channel/codex/adaptor.go.
export const HEADER_PRESETS: {
  name: string
  headers: Record<string, string>
}[] = [
  {
    name: 'Codex',
    headers: { 'User-Agent': 'codex_cli_rs', originator: 'codex_cli_rs' },
  },
  { name: 'DeepSeek Harness', headers: { 'User-Agent': 'DeepSeek Harness' } },
  { name: 'OpenCode', headers: { 'User-Agent': 'opencode' } },
]

/** Replace only preset headers, treating HTTP header names case-insensitively. */
export function mergeHeaderPreset(
  current: string | undefined,
  headers: Record<string, string>
): string {
  if (getHeaderPassthroughState(current) === 'invalid') return current ?? ''
  const existing: Record<string, unknown> = current?.trim()
    ? JSON.parse(current)
    : {}
  const replaced = new Set(Object.keys(headers).map((key) => key.toLowerCase()))
  const merged = Object.fromEntries(
    Object.entries(existing).filter(([key, value]) => {
      const name = key.toLowerCase()
      if (replaced.has(name)) return false
      // Switching away from Codex clears only its known preset identity.
      if (
        !replaced.has('originator') &&
        name === 'originator' &&
        value === 'codex_cli_rs'
      ) {
        return false
      }
      return true
    })
  )
  return JSON.stringify({ ...merged, ...headers }, null, 2)
}
