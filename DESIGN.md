# DESIGN: `dorgu dashboard`

Decided 2026-11-10 with the founder (`/design-consultation`). This is the design system for the dashboard
(`dorgu-ai/dorgu-platform-v1`, React 19 + Vite + Tailwind 4). It lives in this repo and is the living reference:
change it here, in the same pull request as the UI. The decision record is in the internal docs
(`harness/DESIGN-dashboard.md`), which points back to this file.

## Revisions since the decision

Two values in the decided palette failed the contrast rule in section 3.5 once the automated check
(`web/src/design/contrast.test.ts`) ran over every pair, so they were adjusted. Nothing else moved.

- **Light `danger` text** `#e0393f` became `#cf2a30`. The decided value measured 4.35:1 on `surface` and 3.81:1 on
  `danger-soft`, under the 4.5:1 that body text needs. The dark value passes and is unchanged.
- **Input borders** use `ink-faint` (3.3:1 or better against `bg` and `surface` in both themes) instead of
  `line-strong`, which measures about 1.6:1 and cannot be a control boundary under WCAG 1.4.11. `line-strong` stays
  for emphasised dividers.

## 1. Direction

**A calm, precise instrument.** The dashboard is where a platform engineer decides whether a machine is allowed to
change production. It should feel quiet, exact and trustworthy: Linear's restraint, Vercel's monochrome, Grafana's
data density done well, Stripe's care around consequential actions.

- **Independent product look.** It does not reuse the dorgu.run website brand (warm brown, Geist).
- **Monochrome ink base** (from option B): black, white and true greys carry all structure. The ordinary primary
  button is solid ink.
- **One indigo accent** (from option A): it means "you can act here, and this action matters". It appears on focus
  rings, links, the selected nav item and selection states, and on the single consequential action of a screen:
  **Approve** (and the confirm step of Approve). Nothing decorative is indigo.
- **Colour is a fact.** Red, amber and green mean unhealthy, degraded and healthy, and nothing else. Orchid means
  "a machine wrote this text". If a colour appears, it is telling you something.
- **Numbers carry their provenance.** Every figure says where it came from (persona, live Deployment, Prometheus,
  metrics-server, operator status) and uses tabular digits.
- Follows the system theme by default with a toggle; light and dark are both first-class.

## 2. Typography

Bundled in the binary (`@fontsource-variable/inter`, `@fontsource-variable/jetbrains-mono`, both OFL). No font
or asset is ever fetched from the network: the dashboard works offline and makes no third-party requests.

| Role | Family | Why |
|---|---|---|
| UI and body | **Inter** (variable), fallback `ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif` | Very legible at 13 to 14 px; true tabular figures (`tnum`) for dense tables |
| Code, resource names, diffs, commands | **JetBrains Mono** (variable), fallback `ui-monospace, 'SF Mono', Menlo, Consolas, monospace` | Clear `0/O`, `1/l`, wide enough for YAML diffs |

Headings use Inter (no separate display face). Numbers in tables, meters and KPIs always use
`font-variant-numeric: tabular-nums`.

| Token | Size / line height | Weight | Use |
|---|---|---|---|
| `text-kpi` | 28 / 32 | 600 | Overview KPI figures only |
| `text-h1` | 20 / 28 | 600 | Page title |
| `text-h2` | 16 / 24 | 600 | Section title, panel title |
| `text-h3` | 14 / 20 | 600 | Card title |
| `text-body` | 14 / 20 | 400 | Default text |
| `text-table` | 13 / 18 | 400 | Table cells, dense lists |
| `text-small` | 12 / 16 | 400 | Metadata, provenance labels |
| `text-caption` | 11 / 14 | 500 | Badges, gate stage pills, overlines (uppercase +0.04em) |
| `text-code` | 12.5 / 18 | 400 mono | Inline code, diffs, commands |

## 3. Colour

Eight roles: ink neutrals, accent, danger, warn, ok, machine text, plus focus (= accent). Values are hex for review;
the CSS uses the same values.

### 3.1 Neutrals (ink, from option B)

| Token | Light | Dark | Use |
|---|---|---|---|
| `bg` | `#fafafa` | `#0a0a0a` | Page background |
| `surface` | `#ffffff` | `#121212` | Cards, panels, table body |
| `surface-sunken` | `#f3f3f3` | `#0d0d0d` | Code blocks, inset areas, sidebar |
| `surface-hover` | `#f5f5f5` | `#1a1a1a` | Row hover |
| `line` | `#e4e4e4` | `#272727` | Borders, dividers |
| `line-strong` | `#cfcfcf` | `#3a3a3a` | Emphasised dividers (not control borders: see Revisions) |
| `ink` | `#111111` | `#ededed` | Primary text, **primary (non-consequential) button fill** |
| `ink-on` | `#ffffff` | `#0a0a0a` | Text on ink fill |
| `ink-muted` | `#616161` | `#9e9e9e` | Secondary text, provenance |
| `ink-faint` | `#8a8a8a` | `#6e6e6e` | Placeholders, disabled (never for information) |

### 3.2 Accent (indigo, from option A)

| Token | Light | Dark | Use |
|---|---|---|---|
| `accent` | `#4f46e5` | `#7b7bf7` | Approve button, links, focus ring, selected nav, checked controls |
| `accent-on` | `#ffffff` | `#0a0a0a` | Text on accent fill |
| `accent-hover` | `#4338ca` | `#9494fa` | Hover on accent fill |
| `accent-soft` | `#eef0fe` | `#1b1b3a` | Selected row, selected nav background |

### 3.3 Status (reserved: only health and gate outcomes)

| Token | Light | Dark | Soft light | Soft dark | Means |
|---|---|---|---|---|---|
| `danger` | `#cf2a30` | `#ff5257` | `#fdecec` | `#341416` | Unhealthy, refused, failed, destructive |
| `warn` | `#a65300` | `#f2a531` | `#fbefdd` | `#33260f` | Degraded, deferred, expiring, advisory-only |
| `ok` | `#15784b` | `#33c178` | `#e2f3ea` | `#102b1c` | Healthy, passed, verified |

There is deliberately **no info blue**: anything informational is neutral ink. Blue-ish would compete with the
indigo accent.

### 3.4 Machine-written text (orchid)

| Token | Light | Dark | Use |
|---|---|---|---|
| `machine` | `#8e35e6` | `#c58af9` | Label and dashed edge of any text an AI or agent wrote |
| `machine-soft` | `#f4ecfd` | `#241a33` | Background of that block |

### 3.5 Rules

- Status colours never decorate. A red dot is always a fact about the cluster or the gate.
- Every status colour is paired with a word or icon (colour is never the only signal; WCAG 1.4.1).
- Text contrast is at least 4.5:1 for body and 3:1 for large text and UI boundaries, in both themes. P4a-3
  verifies every pair with an automated check.
- Charts: series use neutrals plus the accent; status colours only when the series is a status.

## 4. Spacing, shape, layout

- **Base unit 4 px.** Scale: `1`=4, `2`=8, `3`=12, `4`=16, `5`=20, `6`=24, `8`=32, `10`=40, `12`=48.
- **Radius:** 6 px default (buttons, inputs, cards), 4 px for pills and badges, 8 px for panels and dialogs.
  No fully rounded buttons.
- **Elevation:** borders, not shadows. One soft shadow is allowed for overlays only (dialogs, popovers, toasts).
- **Layout:** fixed left sidebar (232 px, collapsible to 56 px icons), top bar (48 px) with cluster context,
  **authenticated identity**, live/stale indicator and theme toggle. Content is fluid with 24 px gutters. Reading
  surfaces (review panel prose) cap at 720 px.
- **Breakpoints:** `sm` 640, `md` 1024, `lg` 1280, `xl` 1536. Desktop-first; below `md` the sidebar collapses and
  tables become stacked rows. Approve (from a later release) is allowed on any width, with the same review content.

## 5. Components

### 5.1 Buttons

| Variant | Look | Use |
|---|---|---|
| **Approve** | Accent fill, `accent-on` text | The one consequential action: Approve, and its Confirm step. At most one per screen |
| Primary | Ink fill, `ink-on` text | Ordinary primary actions (Apply filter, Copy command, Open) |
| Secondary | Surface, `line-strong` border, ink text | Reject…, Cancel, secondary actions |
| Ghost | Transparent, ink-muted text, hover `surface-hover` | Toolbar and row actions |
| Destructive | Danger border and text; fill only inside its confirm dialog | Delete-like actions (none in P4a; reserved) |

Heights 32 px (default) and 28 px (dense). Focus is a 2 px accent ring offset 2 px, always visible on keyboard focus.
Disabled buttons say why in a tooltip.

### 5.2 Cards and panels

- **Standard:** `surface`, 1 px `line`, radius 6, padding 16.
- **Interactive:** standard plus `surface-hover` on hover and the whole card focusable; the cursor changes only if it
  navigates.
- **Highlighted:** 1 px accent border plus `accent-soft` header (the item currently under review).

### 5.3 Domain components (Dorgu-specific)

- **Gate strip.** Six pills in order: Validate, Ground, Ownership, Guardrails, Mode, Dedup. Each pill is passed (ok
  soft), refused (danger soft, shows reason on hover and in the review panel), deferred (warn soft; "deferred" is not
  a refusal and says so), or not run (neutral outline). A refusal stops the strip; later stages render as not run.
- **Guardrail meter.** Label, a thin bar, and numbers: "change +50% of limit (max 100%)". The bar is ink; it turns
  warn above 80% of the ceiling and danger when over. Always show the ceiling and its source (persona or ClusterPersona).
- **Ownership badge.** `Unmanaged` (neutral) or `Helm` / `Argo CD` / `Flux` / `Kustomize` (warn outline) plus the
  text "advisory only: Dorgu will not write this workload". Owned workloads never show an Approve that implies a write.
- **Ticket chip.** States: not issued (neutral), issued with countdown "expires in 12:41" (accent outline),
  consumed (ok), expired with reason (warn: ApprovalTimedOut, Superseded, PredatesGate, IncidentResolved).
- **Machine text block.** Orchid 2 px dashed left edge, `machine-soft` background, uppercase caption
  "AI explanation · untrusted text" or "Written by agent `<name>` · untrusted text". Rendered as plain text, never
  markdown-to-HTML, never links. Truncated with "Show all" past 6 lines.
- **Provenance label.** `text-small`, ink-muted, after a figure: "from Prometheus · 30s ago", "from persona",
  "from live Deployment".
- **Signal badge.** Signal family (Readiness, RestartLoop, Liveness, CrashLoopBackOff, OOMKilled) plus recurrence
  `r1`, `r2`; links to lineage.
- **Diff.** Mono, two columns on `lg` and up, unified below; removed lines danger-soft, added lines ok-soft (the one
  place status colours stand for "before and after"; label the columns "Running now" and "After approval").

### 5.4 The approve flow (Stripe-style review)

1. "Review & approve" (accent) on a proposal opens the **review panel** (right side sheet on `lg`, full page below).
2. The panel shows, top to bottom: the target workload and ownership badge, the diff, the gate strip, guardrail
   meters, the machine text block, the ticket chip, and **"You are approving as `<identity>`"** from SelfSubjectReview.
3. Footer: Secondary "Reject…" and accent "Approve". Approve opens a confirm step that restates the one-line change
   and identity; the confirm button is accent and requires a deliberate click (no Enter-to-confirm default).
4. Reject requires a reason from the enum (wrong-diagnosis, too-risky, not-now, duplicate, not-needed, other); `other`
   requires a note.
5. After approval the panel stays open and shows the ticket issuing, then the outcome. No optimistic success: the
   UI shows what the cluster reports.

### 5.5 Forms

Inputs 32 px, `surface`, 1 px `ink-faint` border, radius 6, accent focus ring. Select, checkbox and radio use the accent
when checked. Labels above fields, help text in `text-small` ink-muted, errors in danger with the reason in words.

### 5.6 Navigation

Sidebar items: icon + label, `ink-muted`; hover `surface-hover`; selected `accent-soft` background and ink text
with a 2 px accent bar on the left. Order: Overview, Incidents, Proposals, Apps, Memory, Cluster, Agents. Views not
available in this release are listed but disabled with the reason. Breadcrumbs only inside detail pages.

### 5.7 Feedback and states

- **Toast:** bottom-right, surface + shadow, 5 s, never the only record of an outcome.
- **Alert (inline):** soft status background, icon + sentence + what to do.
- **Modal:** only for the confirm step and destructive confirms.
- **Loading:** skeleton rows that match the final layout; no spinners longer than 300 ms without text.
- **Empty states:** the five distinct reasons from the current dashboard (warming, CRDs missing, operator idle,
  genuinely none, **read refused** naming the resource and reason) keep their own screens.
- **Live/stale:** the top bar shows "Live" (ok dot) or "Stale since 14:02" (warn) when the SSE stream drops.

## 6. Motion

- Default: 150 ms `cubic-bezier(0.2, 0, 0, 1)` for hover, focus and colour changes.
- Panels and sheets: 200 ms slide and fade in, 150 ms out. Dialogs: 150 ms fade and 98% to 100% scale.
- Data updates do not animate position. A changed row gets a 600 ms background highlight that fades out.
- Countdown ticks change text only. Nothing pulses or loops.
- `prefers-reduced-motion: reduce` turns every transition into an instant change.

## 7. Copy

- No em dashes. Plain sentences. Say what happened and what to do next.
- Name Dorgu's actions precisely: "Dorgu proposed", "the gate refused (Guardrails: change exceeds 100% of limit)",
  "deferred until the incident is stable", "ticket expired: incident resolved".
- Never call a deferral a refusal, or an advisory step a fix.

## 8. Tokens (Tailwind 4)

```css
@import 'tailwindcss';
@import '@fontsource-variable/inter';
@import '@fontsource-variable/jetbrains-mono';

@custom-variant dark (&:where(.dark, .dark *));

@theme {
  --font-sans: 'Inter Variable', ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif;
  --font-mono: 'JetBrains Mono Variable', ui-monospace, 'SF Mono', Menlo, Consolas, monospace;

  --text-kpi: 28px;   --text-kpi--line-height: 32px;
  --text-h1: 20px;    --text-h1--line-height: 28px;
  --text-h2: 16px;    --text-h2--line-height: 24px;
  --text-h3: 14px;    --text-h3--line-height: 20px;
  --text-body: 14px;  --text-body--line-height: 20px;
  --text-table: 13px; --text-table--line-height: 18px;
  --text-small: 12px; --text-small--line-height: 16px;
  --text-caption: 11px; --text-caption--line-height: 14px;
  --text-code: 12.5px; --text-code--line-height: 18px;

  --radius-pill: 4px; --radius-ctl: 6px; --radius-panel: 8px;
  --ease-standard: cubic-bezier(0.2, 0, 0, 1);

  /* light (default) */
  --color-bg: #fafafa;
  --color-surface: #ffffff;
  --color-surface-sunken: #f3f3f3;
  --color-surface-hover: #f5f5f5;
  --color-line: #e4e4e4;
  --color-line-strong: #cfcfcf;
  --color-ink: #111111;
  --color-ink-on: #ffffff;
  --color-ink-muted: #616161;
  --color-ink-faint: #8a8a8a;
  --color-accent: #4f46e5;
  --color-accent-on: #ffffff;
  --color-accent-hover: #4338ca;
  --color-accent-soft: #eef0fe;
  --color-danger: #cf2a30;  --color-danger-soft: #fdecec;
  --color-warn: #a65300;    --color-warn-soft: #fbefdd;
  --color-ok: #15784b;      --color-ok-soft: #e2f3ea;
  --color-machine: #8e35e6; --color-machine-soft: #f4ecfd;
}

.dark {
  --color-bg: #0a0a0a;
  --color-surface: #121212;
  --color-surface-sunken: #0d0d0d;
  --color-surface-hover: #1a1a1a;
  --color-line: #272727;
  --color-line-strong: #3a3a3a;
  --color-ink: #ededed;
  --color-ink-on: #0a0a0a;
  --color-ink-muted: #9e9e9e;
  --color-ink-faint: #6e6e6e;
  --color-accent: #7b7bf7;
  --color-accent-on: #0a0a0a;
  --color-accent-hover: #9494fa;
  --color-accent-soft: #1b1b3a;
  --color-danger: #ff5257;  --color-danger-soft: #341416;
  --color-warn: #f2a531;    --color-warn-soft: #33260f;
  --color-ok: #33c178;      --color-ok-soft: #102b1c;
  --color-machine: #c58af9; --color-machine-soft: #241a33;
}

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { transition-duration: 0ms !important; animation-duration: 0ms !important; }
}
```

Theme selection: on load, `.dark` follows `prefers-color-scheme` unless the user picked a theme (stored in
`localStorage`); a toggle in the top bar cycles System, Light, Dark.

## 9. For whoever builds UI

- Use tokens only; no raw hex in components. Lint rule: no arbitrary colour values in class names.
- New colours need a reason that fits section 3. "It looks nice" is not one.
- Any text from an LLM or agent goes through the machine text block. No exceptions, including titles and summaries.
- Each view ships with its empty, loading, error and refused states and passes the contrast check.
- Enforcement: `npm run lint` rejects raw hex values and arbitrary or default-palette colour classes in `web/src`;
  `npm test` runs the contrast check over every token pair; the Playwright suite runs axe in both themes.

## 10. How the states are told apart

An empty list has five different causes, and each one is its own screen (section 5.7):

| State | Cause | The screen says |
|---|---|---|
| Warming | The informers have not finished their first list | Reading your cluster; skeleton rows |
| CRDs missing | The API server does not serve the dorgu.io resources | The operator is not installed; the install command |
| Operator idle | The CRDs exist and no operator replica is ready | Nothing is detecting, so empty is not good news |
| None | The operator is running and has recorded nothing, or there is genuinely nothing to list | Quiet, and what that does and does not mean |
| Read refused | The kubeconfig user may not read the resource | Names the resource and the reason |
