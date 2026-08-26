# dorgu-platform-v1

A local, read-only dashboard for what Dorgu sees in your Kubernetes cluster: your
applications, and the incidents detected against them.

## The dashboard can do exactly what you can do, and nothing more

This is the whole security model, and it is short enough to check yourself:

- **It uses your kubeconfig.** The same file `kubectl` reads, the same
  `KUBECONFIG` variable, the same current context.
- **It holds no credentials.** There is no token, no stored secret and no service
  account anywhere in this repo. Search it.
- **It needs no permissions of its own.** No ClusterRole to install, nothing to
  grant. If you cannot list Deployments, neither can the dashboard.
- **It binds to 127.0.0.1.** Nothing outside your machine can reach it. Binding
  elsewhere requires a flag and prints a warning saying what you are doing.
- **It writes nothing.** Every endpoint is a read. There is no mutation in this
  release at all.

If a view looks empty, that is because your cluster is empty of that thing, not
because permission was denied somewhere quietly. The dashboard says which of
those it is on every screen.

## What it shows

| View | State | What it tells you |
|---|---|---|
| **Apps** | shipped | Your ApplicationPersonas **and the Deployments nothing is watching**, with health, who owns each workload, and the live resource block |
| **Incidents** | shipped | A live feed: severity, signal, persona, root cause, confidence and which provider produced it |
| **Remediations** | not built | Gated. AI-planned remediations are not reliably applicable yet, so showing a plan here would imply an action the product cannot take |
| **Cluster** | not built | Gated. Cluster health can currently report impossible CPU figures, and a wrong number in a card looks authoritative in a way terminal output does not |

The two unbuilt views appear in the navigation, disabled, with the reason on
hover. The reason comes from the server, so it cannot drift from what the build
actually does.

### The Apps view lists what Dorgu is blind to

This is the point of it. A dashboard that shows only the apps Dorgu already knows
about would report a healthy cluster while three broken Deployments nobody had
imported sat next to it. So every Deployment with no `ApplicationPersona` gets a
row, marked **not watched**, with the exact `dorgu persona import` command that
fixes it.

Cluster add-on namespaces (`kube-system` and friends) are skipped in the
all-namespaces view, because forty control-plane Deployments would bury three
real apps. Ask for a namespace explicitly with `--namespace` and you get all of
it.

### Health says where it got its answer

Every health reading is labelled with its source:

- **persona-status** is the Dorgu operator's own verdict.
- **observed** is what this process read off the Deployment and its Pods a moment
  ago.

They can disagree, because a persona's status is written on a reconcile interval.
When the operator's verdict is more flattering than what the dashboard can see
right now, the observation wins and the row says **operator says otherwise** with
both readings. A green badge over a live crash loop is the one lie this screen
must not tell.

### The Owner column tells you what Dorgu will do before you ask

`managedBy` is who reconciles a workload's desired state: `helm`, `argocd`,
`flux`, `kustomize`, `unmanaged` or `unknown`. Only **unmanaged** permits the CLI
to patch a Deployment. Everything else, `unknown` included, means Dorgu explains
rather than writes, because patching a Helm-owned workload is what makes the next
`helm upgrade` fail on a field-manager conflict. Hover the badge to see which of
those applies and why.

## Nothing here needs refreshing

There is no refresh button, and that is deliberate rather than an omission.

The server holds long-lived **watches** on the Kubernetes API and keeps an
in-memory cache. Changes are pushed to the browser over **server-sent events**,
and every event is a complete snapshot of one view, so applying it is a
replacement with no merge and no ordering assumption. A new or reconnecting client
is sent a fresh snapshot of every view immediately, so a laptop that slept, a VPN
that dropped or a restarted dashboard all recover on their own.

If you ever find yourself reaching for reload, that is a bug. Report it.

## Install and run

Prerequisites: Go 1.25 or newer, Node 20 or newer, and a kubeconfig.

```bash
make all      # build the SPA, then the binary with it embedded
./bin/dorgu-dashboard
```

Then open the URL it prints, which is `http://127.0.0.1:7171/` by default.

```
Usage:
  dorgu-dashboard [flags]

  --kubeconfig       path to kubeconfig (default: $KUBECONFIG, then ~/.kube/config)
  --context          kubeconfig context to use (default: the current context)
  --namespace        limit every view to one namespace (default: all namespaces)
  --host             bind address (default: 127.0.0.1)
  --port             bind port; 0 asks the kernel for a free one (default: 7171)
  --allow-host       Host header allowlist, required when --host is not loopback
  --incident-limit   cap the incident feed; 0 uses the default, -1 means no cap
  --log-level        debug, info, warn or error (default: info)
```

If your kubeconfig user is scoped to one namespace, pass `--namespace`. A
cluster-wide watch will otherwise be refused and the views will say so.

### As part of the CLI

The dashboard is a Go package first and a binary second, so `dorgu dashboard` can
be one command with nothing extra to install:

```go
import "github.com/dorgu-ai/dorgu-platform-v1/pkg/dashboard"

err := dashboard.Run(ctx, dashboard.Options{
    Kubeconfig: kubeconfigPath,
    Namespace:  namespace,
    OnReady:    func(url string) { fmt.Println("dashboard:", url) },
})
```

`cmd/dorgu-dashboard` is a thin wrapper over that same call, so the standalone
binary and the CLI subcommand cannot behave differently.

## Trying it without a cluster

`hack/devcluster` starts a real Kubernetes API server, installs the dorgu.io
CRDs, and fills it with a brownfield-shaped cluster: a Helm-owned app whose
persona name does not match its Deployment, an unmanaged app, an ArgoCD-owned app
that is degraded, and a Deployment nobody has imported.

It needs the envtest binaries, which the operator repo's `make setup-envtest`
downloads:

```bash
export KUBEBUILDER_ASSETS=$(ls -d ../dorgu-operator/bin/k8s/*)
go run ./hack/devcluster -break checkout

# in another shell, using the kubeconfig it printed
KUBECONFIG=/tmp/dorgu-devcluster.kubeconfig ./bin/dorgu-dashboard
```

Add `-no-crds` to see the operator-not-installed screens.

## Architecture

```
Kubernetes API ──watch──> informers ──> in-memory cache
                                             │
                              ┌──────────────┴──────────────┐
                              │                             │
                        REST (initial read)          SSE (every change)
                              │                             │
                              └────────> browser <──────────┘
                                     (embedded SPA)
```

- **Backend Go**, with `client-go` informers over the five `dorgu.io` CRDs plus
  Deployments and Pods. The HTTP layer reads the cache and never the API server,
  so twenty open tabs cost the API server nothing.
- **SSE, not WebSocket.** Updates are one-directional, the browser reconnects an
  SSE stream by itself, and it is plain HTTP so it inherits the same Host guard
  and content security policy as every other route.
- **The CRD types are imported** from `dorgu-ai/dorgu-operator/api/v1` rather than
  restated, so a CRD gaining a field does not need a change here.
- **`go:embed`** carries the built SPA, so this is one binary with no asset
  directory to deploy beside it.
- **Frontend Vite + React 19 + TypeScript**, with TanStack Router, Query, Table
  and Virtual, and Tailwind v4 with copy-in shadcn-style components. Not Next.js:
  its value is SSR, server components and a Node runtime, all of which are
  discarded the moment a static build goes inside a Go binary.

Incident and app lists are virtualised, so a cluster having a bad hour renders at
the same cost as an idle one.

### Layout

```
cmd/dorgu-dashboard   standalone binary and container entrypoint
pkg/dashboard         the package the dorgu CLI imports
internal/kube         kubeconfig loading, clients, CRD discovery
internal/informers    watches, feeding the cache
internal/store        the in-memory cache
internal/view         pure projections into the payloads the browser renders
internal/httpapi      REST, SSE and the embedded SPA
internal/sse          the event broker and its coalescer
internal/webui        go:embed of the built SPA
internal/workload     persona-to-Deployment matching and ownership detection
web/                  the Vite + React SPA
hack/devcluster       a real API server with fixtures, for local verification
```

`internal/workload` is a port of the operator's own `internal/workload`. Go
forbids importing another module's internal packages, and the dashboard has to
agree with the operator about which Deployment a persona describes and who owns
it, or it will show a different answer than `dorgu health` for the same cluster.
The logic is copied verbatim and pinned by tests that mirror the operator's. The
right fix is for the operator to promote that package to `pkg/`.

## Development

```bash
make check      # everything CI runs: gofmt, vet, race tests, web typecheck, web lint
make web        # build the SPA into internal/webui/dist
make build      # build the binary with whatever is in that directory
make web-dev    # Vite dev server on :5173, proxying /api to a dashboard on :7171
make cover      # test coverage per package
```

`internal/webui/dist` holds a committed `.gitkeep` on purpose: without a file in
it, `go:embed all:dist` matches nothing and `go build` fails on a fresh clone. A
binary built without the frontend still runs, and serves an explanation at `/`
telling you to run `make web`.

## Not in this release

- Remediations and the approve action. Gated on the AI-planned remediations being
  reliably applicable.
- The Cluster view. Gated on cluster health reporting correct saturation figures.
- Authentication. There is none, by design: the dashboard binds loopback and
  holds no credentials, and an unauthenticated dashboard reachable from a network
  would be the single strongest objection anyone could raise against it.
- Telemetry. Nothing here phones home.

## Licence

Apache 2.0. See [LICENSE](./LICENSE).
