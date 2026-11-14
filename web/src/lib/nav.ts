import {
  Activity,
  BookOpen,
  Bot,
  Boxes,
  ClipboardCheck,
  Gauge,
  Server,
} from 'lucide-react'

/**
 * Sidebar items, section 5.6: icon and label, in the order Overview, Incidents,
 * Proposals, Apps, Memory, Cluster, Agents. The route for Proposals is still
 * /remediations, because links to it exist.
 *
 * The server's list decides what is available and why not, so a view this build
 * cannot render shows disabled with its reason and no frontend change.
 */
export const NAV = [
  { id: 'overview', to: '/overview', icon: Gauge, title: 'Overview' },
  { id: 'incidents', to: '/incidents', icon: Activity, title: 'Incidents' },
  { id: 'remediations', to: '/remediations', icon: ClipboardCheck, title: 'Proposals' },
  { id: 'apps', to: '/apps', icon: Boxes, title: 'Apps' },
  { id: 'memory', to: '/memory', icon: BookOpen, title: 'Memory' },
  { id: 'cluster', to: '/cluster', icon: Server, title: 'Cluster' },
  { id: 'agents', to: undefined, icon: Bot, title: 'Agents' },
] as const


/**
 * Prefixes a release version with "v" and leaves anything else alone. An
 * unstamped build reports "dev", and "vdev" reads like a typo.
 */
export function formatVersion(version: string): string {
  return /^\d/.test(version) ? `v${version}` : version
}
