import { useEffect, useState } from 'react'

/** How long a changed row stays highlighted. Matches the CSS animation. */
const FLASH_MS = 700

const NOTHING: ReadonlySet<string> = new Set()

interface FlashState {
  /** The row versions this component last reacted to. */
  versions: Map<string, string>
  /** The rows currently flashing. */
  flashed: ReadonlySet<string>
  /** Bumped on each change so the clear timer restarts. */
  seq: number
}

/**
 * useFlash reports which rows changed since the last snapshot.
 *
 * A live table that updates silently is hard to trust: a reader cannot tell
 * whether nothing is happening or nothing is arriving. One short flash on the
 * rows that actually changed answers that without turning the page into a light
 * show, and the CSS honours prefers-reduced-motion.
 *
 * `version` is what counts as a change. Passing the whole row would flash
 * everything on every push, because the server sends a full snapshot each time
 * and object identity is never stable across one.
 *
 * The comparison happens during render rather than in an effect. That is the
 * pattern React documents for adjusting state when inputs change, and it avoids
 * the cascading render an effect-plus-setState would cause on every push. The
 * only effect here is the timer that clears the highlight.
 */
export function useFlash<T>(
  items: readonly T[],
  id: (item: T) => string,
  version: (item: T) => string,
): ReadonlySet<string> {
  const versions = snapshot(items, id, version)

  const [state, setState] = useState<FlashState>(() => ({
    // Seeded with the first snapshot, because the initial load is not a change.
    // Flashing every row on arrival would make the signal meaningless.
    versions,
    flashed: NOTHING,
    seq: 0,
  }))

  if (!sameVersions(state.versions, versions)) {
    const changed = changedKeys(state.versions, versions)
    setState({
      versions,
      flashed: changed.size > 0 ? changed : NOTHING,
      seq: state.seq + 1,
    })
  }

  useEffect(() => {
    if (state.flashed.size === 0) return
    const timer = setTimeout(() => {
      setState((previous) => ({ ...previous, flashed: NOTHING }))
    }, FLASH_MS)
    return () => clearTimeout(timer)
  }, [state.seq, state.flashed])

  return state.flashed
}

function snapshot<T>(
  items: readonly T[],
  id: (item: T) => string,
  version: (item: T) => string,
): Map<string, string> {
  const out = new Map<string, string>()
  for (const item of items) {
    out.set(id(item), version(item))
  }
  return out
}

function sameVersions(a: Map<string, string>, b: Map<string, string>): boolean {
  if (a.size !== b.size) return false
  for (const [key, value] of b) {
    if (a.get(key) !== value) return false
  }
  return true
}

/** Rows that are new or whose version moved. A removed row has nothing to flash. */
function changedKeys(before: Map<string, string>, after: Map<string, string>): Set<string> {
  const changed = new Set<string>()
  for (const [key, value] of after) {
    if (before.get(key) !== value) changed.add(key)
  }
  return changed
}
