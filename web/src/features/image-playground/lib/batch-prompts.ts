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
 * Batch mode: several prompts submitted at once, split on runs of two or more
 * blank lines. A single blank line stays inside one prompt.
 */
export function splitBatchPrompts(text: string): string[] {
  return text
    .split(/\r?\n[ \t]*\r?\n[ \t]*\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean)
}

/** Run workers with bounded concurrency; stop launching once shouldStop is true. */
export async function runWithConcurrency<T>(
  items: T[],
  limit: number,
  worker: (item: T, idx: number) => Promise<void>,
  shouldStop: () => boolean = () => false
): Promise<void> {
  let next = 0
  const runners = Array.from(
    { length: Math.min(Math.max(1, limit), items.length) },
    async () => {
      while (next < items.length && !shouldStop()) {
        const idx = next
        next += 1
        await worker(items[idx], idx)
      }
    }
  )
  await Promise.all(runners)
}
