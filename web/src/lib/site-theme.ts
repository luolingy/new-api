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
/**
 * Site theme plugin.
 *
 * The administrator configures a background image URL in Site Settings; the
 * backend publishes it through `/api/status` as `site_theme`. This module turns
 * that payload into the `<body>` attributes consumed by `styles/site-theme.css`.
 *
 * The background image is the activation switch: an empty URL removes every
 * theme attribute and leaves the default dashboard untouched.
 */

export type SiteTheme = {
  /** Absolute http(s) URL or root-relative path. Empty disables the theme. */
  backgroundImage: string
  /**
   * CSS background-position keyword pair controlling which region of the image
   * stays visible when the viewport crops it (e.g. a landscape image on a
   * portrait screen). Restricted to the presets below.
   */
  backgroundPosition: string
  /** Frosted card/popover/sidebar surfaces on top of the background image. */
  glassCards: boolean
  /** Opacity (0-100) of the glass panels; lower shows more of the image. */
  cardOpacity: number
}

/** Allowed focus presets mapped to CSS background-position values. */
export const SITE_THEME_BACKGROUND_POSITIONS = [
  'left top',
  'center top',
  'right top',
  'left center',
  'center',
  'right center',
  'left bottom',
  'center bottom',
  'right bottom',
] as const

export type SiteThemeBackgroundPosition =
  (typeof SITE_THEME_BACKGROUND_POSITIONS)[number]

export const DEFAULT_BACKGROUND_POSITION: SiteThemeBackgroundPosition = 'center'

/** Matches the backend default for the glass panel opacity. */
export const DEFAULT_CARD_OPACITY = 68

export const DEFAULT_SITE_THEME: SiteTheme = {
  backgroundImage: '',
  backgroundPosition: DEFAULT_BACKGROUND_POSITION,
  glassCards: true,
  cardOpacity: DEFAULT_CARD_OPACITY,
}

export const SITE_THEME_BACKGROUND_ATTRIBUTE = 'data-site-theme-background'
export const SITE_THEME_GLASS_ATTRIBUTE = 'data-site-theme-glass'
export const SITE_THEME_BACKGROUND_VARIABLE = '--site-theme-background-image'
export const SITE_THEME_POSITION_VARIABLE = '--site-theme-background-position'
export const SITE_THEME_CARD_OPACITY_VARIABLE = '--site-theme-card-opacity'

/** Clamp an arbitrary value to an integer 0-100, defaulting when unset. */
export function normalizeCardOpacity(value: unknown): number {
  const parsed = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(parsed) || parsed <= 0) return DEFAULT_CARD_OPACITY
  return Math.min(100, Math.max(0, Math.round(parsed)))
}

/** Normalize an arbitrary value to a supported focus preset. */
export function normalizeBackgroundPosition(
  value: unknown
): SiteThemeBackgroundPosition {
  if (typeof value !== 'string') return DEFAULT_BACKGROUND_POSITION
  const normalized = value.trim().toLowerCase().replaceAll(/\s+/g, ' ')
  const match = SITE_THEME_BACKGROUND_POSITIONS.find((p) => p === normalized)
  return match ?? DEFAULT_BACKGROUND_POSITION
}

/**
 * Reject anything that cannot be placed safely inside a CSS `url("...")` token
 * or that would load a non-http resource. Mirrors the backend validation in
 * `setting/theme_setting`; the client re-checks so a stale or hand-edited
 * status payload cannot inject CSS.
 */
export function sanitizeBackgroundImage(value: unknown): string {
  if (typeof value !== 'string') return ''
  const trimmed = value.trim()
  if (trimmed === '') return ''
  if (/[\s"'()\\<>]/.test(trimmed)) return ''
  if (trimmed.startsWith('//')) return ''
  if (trimmed.startsWith('/')) return trimmed
  try {
    const parsed = new URL(trimmed)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return ''
    return trimmed
  } catch {
    return ''
  }
}

function toGlassCards(value: unknown): boolean {
  if (typeof value === 'boolean') return value
  if (value === 'true') return true
  if (value === 'false') return false
  return DEFAULT_SITE_THEME.glassCards
}

/** Normalize the `site_theme` field of `/api/status`. */
export function readSiteTheme(
  status: Record<string, unknown> | null | undefined
): SiteTheme {
  const raw = status?.site_theme
  if (!raw || typeof raw !== 'object') return DEFAULT_SITE_THEME
  const payload = raw as Record<string, unknown>
  return {
    backgroundImage: sanitizeBackgroundImage(payload.background_image),
    backgroundPosition: normalizeBackgroundPosition(payload.background_position),
    glassCards: toGlassCards(payload.glass_cards),
    cardOpacity: normalizeCardOpacity(payload.card_opacity),
  }
}

/**
 * Mirror the theme onto `<body>`. Idempotent: it only adds/removes the
 * attributes and the background variable, so repeated status refreshes are
 * cheap.
 */
export function applySiteTheme(theme: SiteTheme, doc?: Document): void {
  if (typeof document === 'undefined' && !doc) return
  const target = doc ?? document
  const body = target.body
  if (!body) return

  if (theme.backgroundImage) {
    body.setAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE, 'true')
    body.style.setProperty(
      SITE_THEME_BACKGROUND_VARIABLE,
      `url("${theme.backgroundImage}")`
    )
    body.style.setProperty(
      SITE_THEME_POSITION_VARIABLE,
      normalizeBackgroundPosition(theme.backgroundPosition)
    )
    body.style.setProperty(
      SITE_THEME_CARD_OPACITY_VARIABLE,
      String(normalizeCardOpacity(theme.cardOpacity))
    )
  } else {
    body.removeAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)
    body.style.removeProperty(SITE_THEME_BACKGROUND_VARIABLE)
    body.style.removeProperty(SITE_THEME_POSITION_VARIABLE)
    body.style.removeProperty(SITE_THEME_CARD_OPACITY_VARIABLE)
  }

  if (theme.backgroundImage && theme.glassCards) {
    body.setAttribute(SITE_THEME_GLASS_ATTRIBUTE, 'true')
  } else {
    body.removeAttribute(SITE_THEME_GLASS_ATTRIBUTE)
  }
}

/** Apply the theme carried by a `/api/status` payload. */
export function applySiteThemeFromStatus(
  status: Record<string, unknown> | null | undefined,
  doc?: Document
): void {
  applySiteTheme(readSiteTheme(status), doc)
}
