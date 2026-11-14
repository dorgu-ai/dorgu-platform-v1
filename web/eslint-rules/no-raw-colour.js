/**
 * Bans raw colour in components, which is DESIGN.md section 9: "use tokens only;
 * no raw hex in components".
 *
 * Three things are rejected, in any string or template literal:
 *   - a hex colour (#fff, #4f46e5, #4f46e5cc);
 *   - a colour function (rgb(), hsl(), oklch() and friends);
 *   - a Tailwind colour utility that is not a token: an arbitrary value such as
 *     `bg-[#123456]` or `text-[color:var(--x)]`, a default-palette colour such as
 *     `bg-red-500`, or `bg-white` and `text-black`.
 *
 * Token classes (`bg-surface`, `text-ink-muted`, `border-danger/40`) pass. The
 * stylesheet is where colours are defined and is not linted.
 */

const COLOUR_UTILITIES =
  'bg|text|border|border-[xytblrse]|ring|ring-offset|fill|stroke|from|via|to|divide|outline|decoration|shadow|accent|caret|placeholder'

const PALETTE =
  'slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose'

const HEX = /#[0-9a-fA-F]{8}\b|#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3,4}\b/
const COLOUR_FUNCTION = /\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(/
const COLOUR_CLASS = new RegExp(
  `(?:^|[\\s"'\`:!])(?:${COLOUR_UTILITIES})-(?:\\[[^\\]]*(?:#|rgb|hsl|oklch|oklab|lab\\(|lch|color)[^\\]]*\\]|(?:${PALETTE})-\\d{2,3}|white|black)(?![\\w-])`,
)

function violation(text) {
  if (HEX.test(text)) return 'a raw hex colour'
  if (COLOUR_FUNCTION.test(text)) return 'a raw colour function'
  const match = COLOUR_CLASS.exec(text)
  if (match) return `the colour class "${match[0].trim().replace(/^["'`:!]/, '')}", which is not a token`
  return null
}

/** @type {import('eslint').Rule.RuleModule} */
export const noRawColour = {
  meta: {
    type: 'problem',
    docs: { description: 'Disallow raw colour values and non-token colour classes (DESIGN.md section 9).' },
    schema: [],
    messages: {
      raw: 'Found {{what}}. Use a design token (see DESIGN.md section 3 and src/styles.css), or add a token with a reason.',
    },
  },
  create(context) {
    return {
      Literal(node) {
        if (typeof node.value !== 'string') return
        const what = violation(node.value)
        if (what) context.report({ node, messageId: 'raw', data: { what } })
      },
      TemplateElement(node) {
        const what = violation(node.value.cooked ?? node.value.raw)
        if (what) context.report({ node, messageId: 'raw', data: { what } })
      },
    }
  },
}

export default { rules: { 'no-raw-colour': noRawColour } }
