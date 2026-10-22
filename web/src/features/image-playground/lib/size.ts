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
export const IMAGE_SIZE_EDGE_MIN = 64
export const IMAGE_SIZE_EDGE_MAX = 4096

export type ImageSizeTier = {
  id: '1K' | '2K' | '4K'
  sizes: string[]
}

export const IMAGE_SIZE_TIERS: ImageSizeTier[] = [
  {
    id: '1K',
    sizes: [
      '1024x1024',
      '1536x1024',
      '1024x1536',
      '1280x720',
      '720x1280',
      '1024x768',
      '768x1024',
      '1280x544',
    ],
  },
  {
    id: '2K',
    sizes: [
      '2048x2048',
      '2160x1440',
      '1440x2160',
      '2560x1440',
      '1440x2560',
      '2048x1536',
      '1536x2048',
      '2560x1088',
    ],
  },
  {
    id: '4K',
    sizes: [
      '2880x2880',
      '3456x2304',
      '2304x3456',
      '3840x2160',
      '2160x3840',
      '3200x2400',
      '2400x3200',
      '3840x1600',
    ],
  },
]

export function parseImageSize(
  size: string
): { width: number; height: number } | null {
  const match = /^(\d{2,5})x(\d{2,5})$/.exec(size.trim())
  if (!match) return null
  return { width: Number(match[1]), height: Number(match[2]) }
}

export function isValidImageSize(size: string): boolean {
  if (size === 'auto') return true
  const parsed = parseImageSize(size)
  if (!parsed) return false
  return (
    parsed.width >= IMAGE_SIZE_EDGE_MIN &&
    parsed.width <= IMAGE_SIZE_EDGE_MAX &&
    parsed.height >= IMAGE_SIZE_EDGE_MIN &&
    parsed.height <= IMAGE_SIZE_EDGE_MAX
  )
}

export function formatImageSizeLabel(size: string): string {
  const parsed = parseImageSize(size)
  if (!parsed) return size
  return `${parsed.width}×${parsed.height}`
}

export function formatAspectRatio(size: string): string {
  const parsed = parseImageSize(size)
  if (!parsed) return ''
  const divisor = greatestCommonDivisor(parsed.width, parsed.height)
  return `${parsed.width / divisor}:${parsed.height / divisor}`
}

function greatestCommonDivisor(a: number, b: number): number {
  let left = a
  let right = b
  while (right !== 0) {
    const remainder = left % right
    left = right
    right = remainder
  }
  return left || 1
}
