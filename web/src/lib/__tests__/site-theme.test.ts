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
import { afterEach, describe, expect, test } from 'vitest'

import {
  applySiteTheme,
  DEFAULT_SITE_THEME,
  normalizeBackgroundPosition,
  normalizeCardOpacity,
  readSiteTheme,
  sanitizeBackgroundImage,
  SITE_THEME_BACKGROUND_ATTRIBUTE,
  SITE_THEME_BACKGROUND_VARIABLE,
  SITE_THEME_CARD_OPACITY_VARIABLE,
  SITE_THEME_GLASS_ATTRIBUTE,
  SITE_THEME_POSITION_VARIABLE,
} from '@/lib/site-theme'

afterEach(() => {
  document.body.removeAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)
  document.body.removeAttribute(SITE_THEME_GLASS_ATTRIBUTE)
  document.body.style.removeProperty(SITE_THEME_BACKGROUND_VARIABLE)
  document.body.style.removeProperty(SITE_THEME_POSITION_VARIABLE)
  document.body.style.removeProperty(SITE_THEME_CARD_OPACITY_VARIABLE)
})

describe('sanitizeBackgroundImage', () => {
  test.each([
    ['https://cdn.example.com/bg.png', 'https://cdn.example.com/bg.png'],
    [' http://127.0.0.1:8080/bg.jpg?v=2 ', 'http://127.0.0.1:8080/bg.jpg?v=2'],
    ['/assets/bg.webp', '/assets/bg.webp'],
    ['', ''],
  ])('keeps %s unchanged', (value, expected) => {
    expect(sanitizeBackgroundImage(value)).toBe(expected)
  })

  test.each([
    'javascript:alert(1)',
    'data:image/png;base64,AAAA',
    '//evil.example.com/bg.png',
    'https://cdn.example.com/a.png") , url(evil',
    'ftp://cdn.example.com/bg.png',
    'not a url',
  ])('rejects %s', (value) => {
    expect(sanitizeBackgroundImage(value)).toBe('')
  })
})

describe('readSiteTheme', () => {
  test('reads the site_theme payload from /api/status', () => {
    expect(
      readSiteTheme({
        site_theme: {
          background_image: 'https://cdn.example.com/bg.png',
          background_position: 'right top',
          glass_cards: false,
        },
      })
    ).toEqual({
      backgroundImage: 'https://cdn.example.com/bg.png',
      backgroundPosition: 'right top',

      glassCards: false,
      cardOpacity: 68,
    })
  })

  test('falls back to the default theme when the payload is missing', () => {
    expect(readSiteTheme(undefined)).toEqual(DEFAULT_SITE_THEME)
    expect(readSiteTheme({})).toEqual(DEFAULT_SITE_THEME)
    expect(readSiteTheme({ site_theme: 'glass' })).toEqual(DEFAULT_SITE_THEME)
  })

  test('accepts string booleans from a legacy option payload', () => {
    expect(
      readSiteTheme({
        site_theme: { background_image: '', glass_cards: 'false' },
      }).glassCards
    ).toBe(false)
  })
})

describe('applySiteTheme', () => {
  test('marks the body and exposes the background image variable', () => {
    applySiteTheme({
      backgroundImage: 'https://cdn.example.com/bg.png',
      backgroundPosition: 'center',

      glassCards: true,
      cardOpacity: 68,
    })

    expect(document.body.getAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)).toBe(
      'true'
    )
    expect(document.body.getAttribute(SITE_THEME_GLASS_ATTRIBUTE)).toBe('true')
    expect(
      document.body.style.getPropertyValue(SITE_THEME_BACKGROUND_VARIABLE)
    ).toContain('https://cdn.example.com/bg.png')
    expect(
      document.body.style.getPropertyValue(SITE_THEME_POSITION_VARIABLE)
    ).toBe('center')
    expect(
      document.body.style.getPropertyValue(SITE_THEME_CARD_OPACITY_VARIABLE)
    ).toBe('68')
  })

  test('applies a custom focus position', () => {
    applySiteTheme({
      backgroundImage: '/assets/bg.webp',
      backgroundPosition: 'left bottom',

      glassCards: true,
      cardOpacity: 68,
    })

    expect(
      document.body.style.getPropertyValue(SITE_THEME_POSITION_VARIABLE)
    ).toBe('left bottom')
  })

  test('keeps the background image but drops glass surfaces when disabled', () => {
    applySiteTheme({
      backgroundImage: '/assets/bg.webp',
      backgroundPosition: 'center',

      glassCards: false,
      cardOpacity: 68,
    })

    expect(document.body.getAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)).toBe(
      'true'
    )
    expect(document.body.getAttribute(SITE_THEME_GLASS_ATTRIBUTE)).toBeNull()
  })

  test('clears every attribute when the background image is empty', () => {
    applySiteTheme({
      backgroundImage: 'https://cdn.example.com/bg.png',
      backgroundPosition: 'right top',

      glassCards: true,
      cardOpacity: 68,
    })

    applySiteTheme({
      backgroundImage: '',
      backgroundPosition: 'center',

      glassCards: true,
      cardOpacity: 68,
    })

    expect(
      document.body.getAttribute(SITE_THEME_BACKGROUND_ATTRIBUTE)
    ).toBeNull()
    expect(document.body.getAttribute(SITE_THEME_GLASS_ATTRIBUTE)).toBeNull()
    expect(
      document.body.style.getPropertyValue(SITE_THEME_BACKGROUND_VARIABLE)
    ).toBe('')
    expect(
      document.body.style.getPropertyValue(SITE_THEME_CARD_OPACITY_VARIABLE)
    ).toBe('')
  })
})

describe('normalizeCardOpacity', () => {
  test.each([
    [68, 68],
    [0, 68],
    [-5, 68],
    [30, 30],
    [100, 100],
    [101, 100],
    [55.6, 56],
    [undefined, 68],
    ['42', 42],
    ['abc', 68],
  ])('normalizes %s to %s', (input, expected) => {
    expect(normalizeCardOpacity(input)).toBe(expected)
  })
})

describe('normalizeBackgroundPosition', () => {
  test.each([
    ['center', 'center'],
    ['  LEFT  TOP ', 'left top'],
    ['right bottom', 'right bottom'],
    ['20% 80%', 'center'],
    ['', 'center'],
    [undefined, 'center'],
    ['url(evil)', 'center'],
  ])('normalizes %s to %s', (input, expected) => {
    expect(normalizeBackgroundPosition(input)).toBe(expected)
  })
})
