import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { isModelAuthored, machineLabel, needsCollapse } from '@/lib/machine'

import { MachineText, MachineTextInline } from './machine-text'

describe('the machine text block', () => {
  it('captions AI text as untrusted', () => {
    render(<MachineText text="The pod ran out of memory." />)
    expect(screen.getByText('AI explanation · untrusted text')).toBeInTheDocument()
  })

  it('names the agent that wrote it', () => {
    render(<MachineText source={{ kind: 'agent', name: 'triage-bot' }} text="x" />)
    expect(screen.getByText('Written by agent triage-bot · untrusted text')).toBeInTheDocument()
  })

  it('prefixes a label when the block is about one thing', () => {
    render(<MachineText label="Step 1" text="x" />)
    expect(screen.getByText('Step 1 · AI explanation · untrusted text')).toBeInTheDocument()
  })

  it('renders markup as text: nothing is interpreted, and there are no links', () => {
    const hostile = '<img src=x onerror=alert(1)> **bold** [click](https://evil.example) <a href="https://evil.example">link</a> https://evil.example'
    const { container } = render(<MachineText text={hostile} />)
    expect(screen.getByText(hostile)).toBeInTheDocument()
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('a')).toBeNull()
    expect(container.querySelector('strong')).toBeNull()
    expect(container.querySelectorAll('[onerror]')).toHaveLength(0)
  })

  it('keeps line breaks', () => {
    render(<MachineText text={'one\ntwo'} />)
    expect(screen.getByText(/one/).className).toContain('whitespace-pre-wrap')
  })

  it('does not collapse short text, and offers no control for it', () => {
    render(<MachineText text="short" />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('collapses past six lines and shows all on request', async () => {
    const long = Array.from({ length: 8 }, (_, i) => `line ${i + 1}`).join('\n')
    render(<MachineText text={long} />)
    const paragraph = screen.getByText(/line 1/)
    expect(paragraph.className).toContain('line-clamp-6')

    await userEvent.click(screen.getByRole('button', { name: 'Show all' }))
    expect(paragraph.className).not.toContain('line-clamp-6')
    expect(screen.getByRole('button', { name: 'Show less' })).toHaveAttribute('aria-expanded', 'true')

    await userEvent.click(screen.getByRole('button', { name: 'Show less' }))
    expect(paragraph.className).toContain('line-clamp-6')
  })

  it('collapses one very long paragraph too', () => {
    render(<MachineText text={'word '.repeat(200)} />)
    expect(screen.getByRole('button', { name: 'Show all' })).toBeInTheDocument()
  })

  it('has an inline form that keeps the caption for screen readers and the full text in the title', () => {
    render(<MachineTextInline text="Out of memory." />)
    const inline = screen.getByTestId('machine-text-inline')
    expect(inline).toHaveAttribute('title', 'AI explanation · untrusted text: Out of memory.')
    expect(screen.getByText('AI explanation · untrusted text')).toHaveClass('sr-only')
  })
})

describe('machine text decisions', () => {
  it('collapses at exactly more than six lines', () => {
    expect(needsCollapse('a\nb\nc\nd\ne\nf')).toBe(false)
    expect(needsCollapse('a\nb\nc\nd\ne\nf\ng')).toBe(true)
  })

  it('labels both sources', () => {
    expect(machineLabel({ kind: 'ai' })).toContain('AI explanation')
    expect(machineLabel({ kind: 'agent', name: 'x' })).toContain('agent x')
  })

  it('tells a model from Dorgu own rules', () => {
    for (const p of ['ai-enhanced', 'ai-anthropic', 'anthropic', 'openai', 'LLM', 'claude-sonnet']) {
      expect(isModelAuthored(p), p).toBe(true)
    }
    for (const p of ['rule-engine', 'rule-based', 'deterministic', '', undefined]) {
      expect(isModelAuthored(p), String(p)).toBe(false)
    }
  })
})
