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

/** Square canvas size used for the avatar preview and the final upload. */
export const AVATAR_VIEWPORT_SIZE = 288
export const AVATAR_OUTPUT_SIZE = 512
export const AVATAR_MAX_UPLOAD_BYTES = 1024 * 1024

export interface ImageSize {
  width: number
  height: number
}

export interface CropViewport {
  size: number
  zoom: number
  offsetX: number
  offsetY: number
}

export interface CropRect {
  x: number
  y: number
  width: number
  height: number
}

/**
 * Scale that makes the image cover the square viewport at zoom 1.
 */
export function coverScale(image: ImageSize, viewportSize: number): number {
  if (image.width <= 0 || image.height <= 0 || viewportSize <= 0) {
    return 1
  }
  return Math.max(viewportSize / image.width, viewportSize / image.height)
}

/**
 * Clamp a pan offset so the image never exposes empty space inside the viewport.
 */
export function clampCropOffset(
  offset: number,
  scaledLength: number,
  viewportSize: number
): number {
  const overflow = Math.max(0, (scaledLength - viewportSize) / 2)
  return Math.min(overflow, Math.max(-overflow, offset))
}

/**
 * Rect of the source image (in viewport pixels) that fills the square viewport.
 */
export function computeCropRect(
  image: ImageSize,
  viewport: CropViewport
): CropRect {
  const scale = coverScale(image, viewport.size) * Math.max(1, viewport.zoom)
  const width = image.width * scale
  const height = image.height * scale
  const offsetX = clampCropOffset(viewport.offsetX, width, viewport.size)
  const offsetY = clampCropOffset(viewport.offsetY, height, viewport.size)
  return {
    x: (viewport.size - width) / 2 + offsetX,
    y: (viewport.size - height) / 2 + offsetY,
    width,
    height,
  }
}
