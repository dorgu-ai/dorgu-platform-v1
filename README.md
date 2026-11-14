# dorgu-platform-v1

A local, read-only dashboard for what Dorgu sees in your Kubernetes cluster: your
applications, the incidents detected against them, the remediations proposed for
those incidents, the operator's incident memory, the cluster underneath all of it,
and an Overview of what Dorgu detected, proposed and healed, computed by the same
code as `dorgu report`.

## The dashboard can do exactly what you can do, and nothing more

This is the whole security model, and it is short enough to check yourself:

- **It uses your kubeconfig.** The same file `kubectl` reads, the same
  `KUBECONFIG` variable, the same current context.
- **It holds no Kubernetes credentials.** No stored secret and no service account
  anywhere in this repo. Search it. The one secret it does create is a random
  per-run session token, described below, which dies with the process.
- **It needs no permissions of its own.** No ClusterRole to install, nothing to
  grant. If you cannot list Deployments, neither can the dashboard.
- **It binds to 127.0.0.1 on a random free port.** Nothing outside your machine can
  reach it. Binding elsewhere requires a flag and prints a warning saying what you
  are doing.
- **Every API call needs a per-session token.** It is random, 256 bits, generated
  at startup, and in the URL the dashboard prints and opens. Loopback keeps the
  network out; the token keeps out everything else on your machine that can open a
  socket, including a web page in another tab. See [Session security](#session-security).
- **It writes nothing.** Every endpoint is a read. There is no mutation in this
  release at all. The Proposals view shows you a plan and hands you the CLI
  command that can act on it; it cannot approve or apply anything itself, and
  approving from the dashboard arrives in a later release.

An empty view is never silent about why. There are five reasons a list can be
empty and they are five different screens: the caches are still warming, the
operator's CRDs are not installed, **the operator is installed and nothing is
running** (so empty says nothing about your cluster), the operator is running and
has recorded nothing (or you genuinely have none of that thing), or **the read was
refused**. The last one names the resource and the reason, because it is the only
one you have to act on and the only one that will not resolve on its own.

## What it shows

| View | State | What it tells you |
|---|---|---|
| **Overview** | shipped | The KPI tiles from `pkg/report`, each with where it came from and over what window, open incidents, and the proposals waiting or expired. A figure with no sample says **no data**, never 0 |
| **Incidents** | shipped | A live feed: severity, a signal badge with its recurrence (`r1`, `r2`) that opens the lineage, escalation, root cause, and **refusals kept apart from deferrals** |
| **Proposals** (route `/remediations`) | shipped, read-only | The plan, and a review panel with the gate strip, the guardrail meters with their ceilings, the ownership badge, the ticket chip, the diff against the running container and the CLI command that can act. No approve button |
| **Apps** | shipped | Your ApplicationPersonas **and the Deployments nothing is watching**, with health, who owns each workload, and the live resource block |
| **Memory** | shipped | Every IncidentMemory, open or closed, with its conditions: `ProposalRefused` (the live state) next to `ProposalRefusalRecorded` (the durable record) |
| **Cluster** | shipped | Nodes, capacity, add-ons and saturation, with **requested and used as two figures** and every number labelled with where it came from |
| **Agents** | not in this release | Listed and disabled, with the reason. Connected agents arrive with the agent interface |

Every view reports its own state to the browser, so what the navigation says can
never drift from what the build does. A view this release cannot render would
appear disabled with the reason beside it, which is how Agents appears now. A view
that renders and still cannot do something says so on hover instead, which is how
the Proposals screen tells you it has no approve button before you look for one.

### The Overview is `dorgu report`, computed in the browser's server

The numbers on the Overview are not a second implementation. Both the CLI and the
dashboard call `pkg/report`, a pure package over the operator's `api/v1` types, a
clock and a window. The CLI reads through kubectl and converts; the dashboard reads
its informer cache. The same records give the same numbers, and where an input is
missing (the Events the CLI writes its declines to, for example) the report says
**no data** or **not recorded** rather than showing a zero.

Window defaults to 7 days, as `dorgu report` does; `24h` and `30d` are one click
away, and any window `dorgu report --since` accepts works as `/overview?since=`.

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

### The Remediations view keeps Dorgu's arithmetic apart from the model's prose

This is the point of the screen, and it exists because of a specific failure.

A remediation plan mixes two kinds of statement. A model may have written the
root cause, the plan summary and a step's rationale: those are hypotheses with a
confidence attached. Everything in `spec.steps[].safety` is Dorgu measuring a
live workload: the field, the baseline it measured against, the ratio, the
ceiling, and the value that will actually be applied.

Those used to arrive as one paragraph. The guardrail's verdict was a
`[safety:blast-radius] …` prefix spliced onto the front of the model's rationale,
and in clean-room run #4 that put Dorgu's measurement one line below the model's
claim that the same 16x change was *"well within a 2x ceiling"*, with nothing to
tell the reader which of the two had been computed.

So on this screen a rationale is attributed italic prose, and a verdict is a
bordered panel headed **"Dorgu guardrails (Dorgu's measurement, not the plan's)"**
with a monospace fact grid under it. You should be able to tell them apart
without reading either.

**A value a guardrail refused is never shown as a change.** A refused field is
removed from the step's patch, and every diff on this screen is built from the
patch, so the verdict panel is the only place you learn the value was asked for
at all. `applying nothing` is stated rather than left for you to infer from a
missing word.

Two diffs are shown per plan, and the second is the one that matters:

- **ApplicationPersona change** is what Dorgu records.
- **Deployment change** is what happens to the container that is actually
  OOMing, compared against the resources Dorgu observed on it. A review that
  compared persona to persona showed nothing at all for a remediation that
  introduced a resource key the container had never had.

A step's suggested `kubectl` command is filtered twice before it reaches the copy
button: once for shape, because it may have been authored by a model, and once
for ownership, because a command that writes to a workload Helm or ArgoCD owns
would break their next apply. When a command is withheld the screen says so
rather than dropping it silently.

### The Cluster view computes saturation itself

Cluster health once reported **1689% CPU** on a cluster where 25% was requested
and 1% was in use. The figure was
`ClusterPersona.status.resourceSummary.cpuUtilization` rendered verbatim, and the
operator was summing the resource requests of pods no node had accepted. A pod in
the scheduling queue holds no allocation anywhere, and because it can request
more than the cluster owns, the error had no upper bound.

Operator v0.11.0 fixed that field and CLI v0.12.0 stopped depending on it. This
view goes one step further and does the arithmetic itself, over the live Node and
Pod lists it already watches. That is deliberate: the field is written on a
reconcile interval by whatever operator version happens to be installed, and a
card is read as authoritative in a way terminal output is not. It also means
saturation appears with **no operator installed at all**.

Three habits follow from that, and they are visible in every figure:

- **Requested and used are two numbers, never one.** Requests are what the
  scheduler has committed; used is what the containers are consuming. On the
  cluster that produced the 1689% those were 25% and 1%, which is the difference
  between "nothing more will schedule" and "what is running is struggling".
- **A missing used figure says why.** metrics-server is not installed by default
  on any managed Kubernetes offering, so its absence is the ordinary case. It
  renders as `n/a (metrics-server did not answer: …)` with no bar at all, because
  a zero-length bar reads as 0% and nobody measured it. Requested reports either
  way.
- **Every figure says where it came from.** `observed` means this process read it
  off live Nodes and Pods. `persona-status` means the operator wrote it, on a
  reconcile interval. The vocabulary is the Apps view's, so it only has to be
  learned once.

Pods no node has accepted are excluded from the figures and **counted in the
header**, because they are the real problem the old percentage was burying.

The one add-on the dashboard can check first-hand is metrics-server, and when the
operator records it as installed while this process is getting nothing from it,
the row says so. That is the same judgement the Apps view makes about a stale
`Healthy` over a live crash loop.

## Nothing here needs refreshing

There is no refresh button, and that is deliberate rather than an omission.

The server holds long-lived **watches** on the Kubernetes API and keeps an
in-memory cache. Changes are pushed to the browser over **server-sent events**,
and every event is a complete snapshot of one view, so applying it is a
replacement with no merge and no ordering assumption. A new or reconnecting client
is sent a fresh snapshot of every view immediately, so a laptop that slept, a VPN
that dropped or a restarted dashboard all recover on their own.

There is exactly one exception in the whole process, and it is not a watch that
could have been: **node usage is polled**, every thirty seconds, because the
`metrics.k8s.io` API serves `get` and `list` and no `watch` verb at all. There is
no stream to subscribe to. The Cluster view labels those figures with their age
rather than presenting a polled number as a live one.

If you ever find yourself reaching for reload, that is a bug. Report it.

## Install and run

Prerequisites: Go 1.26 or newer, Node 20.19 or newer (22.12 or newer on the 22 line; CI and the Dockerfile use 22), and a kubeconfig.

```bash
make all      # build the SPA, then the binary with it embedded
./bin/dorgu-dashboard
```

Then open the URL it prints. It looks like `http://127.0.0.1:41873/?token=...`: a
random free port and a session token, both new on every start.

```
Usage:
  dorgu-dashboard [flags]

  --kubeconfig       path to kubeconfig (default: $KUBECONFIG, then ~/.kube/config)
  --context          kubeconfig context to use (default: the current context)
  --namespace        limit every view to one namespace (default: all namespaces)
  --host             bind address (default: 127.0.0.1)
  --port             bind port; 0 picks a free one (default: 0)
  --allow-host       Host header allowlist, required when --host is not loopback
  --incident-limit   cap the incident feed; 0 uses the default, -1 means no cap
  --remediation-limit  cap the remediation list; 0 uses the default, -1 means no cap
  --metrics-interval how often to read node usage from metrics-server (default: 30s)
  --log-level        debug, info, warn or error (default: info)
```

If your kubeconfig user is scoped to one namespace, pass `--namespace`. A
cluster-wide watch will otherwise be refused and the views will say so: a
resource the dashboard could not read at all is named on screen with the reason,
rather than leaving a loading skeleton on a read that already failed.

`--namespace` scopes the Apps, Incidents, Proposals, Memory and Overview views. It deliberately
does not scope the Cluster view: saturation is a property of the whole cluster,
and summing one namespace's requests against every node's allocatable would
produce a figure that is wrong in a way nobody could detect from the screen.
Nodes are cluster-scoped, so a namespace-scoped user cannot read them at all and
the Cluster view says that instead of showing an empty table.

### Session security

The dashboard can show everything your kubeconfig can read, so it is locked down
the way a local server that may one day approve things should be:

1. **Loopback, random port.** `127.0.0.1` and a port the kernel picks. `--port`
   fixes it if you want a stable one.
2. **A per-session token.** Generated at startup from `crypto/rand`, and part of
   the URL that is printed and opened. The first request carries it in the query
   string; the server answers with a redirect to the same URL **without** it, and
   sets an `HttpOnly`, `SameSite=Strict` cookie named `dorgu_session_<port>`. The token is therefore not left in
   the address bar, the history or a `Referer` header. A bad token is a `401` and
   sets nothing.
3. **Every `/api` route needs it**, the live stream included: the cookie, or an
   `Authorization: Bearer <token>` header for a script such as `curl`. A token in
   an API query string is not accepted, because it would end up in logs. The embedded
   SPA bundle and `/healthz` are served without it; neither holds cluster data.
4. **Host and Origin checks on every request.** The `Host` header must be on the
   allowlist (`localhost`, `127.0.0.1`, `[::1]`), which is the DNS-rebinding
   defence. Any request that carries an `Origin` header must be same-origin with
   its `Host`, on any method. No CORS headers are ever sent.
5. **Ready for write routes.** All of the above wrap the whole route table rather
   than being attached per route, and a state-changing request authenticated by the
   cookie must also carry an `Origin`. The tests put a `POST`/`PUT`/`PATCH`/`DELETE`
   route behind the same chain, so a future write endpoint inherits checks that are
   already proven.

**What the cookie does not isolate.** Browsers scope cookies by host, not by port.
That has two consequences, stated here because they are true:

- The cookie name carries the port, so two dashboards on `127.0.0.1` (one per
  cluster context, say) keep separate sessions instead of overwriting each other.
- The browser still sends every `127.0.0.1` cookie to every `127.0.0.1` port. If
  you visit some *other* local server in the same browser, it receives this
  dashboard's session cookie and with it the token. `HttpOnly` keeps script from
  reading it and `SameSite=Strict` keeps other sites from using it, but neither
  stops a different server on the same host from being sent it. The token is random,
  per run and unguessable, and dies when the dashboard stops, so the exposure is
  limited to a hostile or careless server already running on your machine that you
  choose to browse to. That is accepted for a localhost tool. Stop the dashboard
  when you are done, and do not browse untrusted local servers in the same browser
  profile while it runs. The `Authorization: Bearer` path used by scripts has no
  such exposure, because nothing attaches it automatically.

A non-loopback `--host` still needs `--allow-host`, and prints a loud warning: the
token then crosses the network in clear text over HTTP and is the only thing
between your cluster and the reader. Put TLS in front of it.

The header also shows **who the cluster thinks you are**, from a
`SelfSubjectReview` (Kubernetes 1.28 or newer). It is the identity every read is
made as. On an older or locked-down cluster it says the identity is unavailable.

### As part of the CLI

The dashboard is a Go package first and a binary second, so `dorgu dashboard` can
be one command with nothing extra to install:

```go
import "github.com/dorgu-ai/dorgu-platform-v1/pkg/dashboard"

err := dashboard.Run(ctx, dashboard.Options{
    Kubeconfig: kubeconfigPath,
    Namespace:  namespace,
    OnReady:    func(url string) { fmt.Println("dashboard:", url) }, // carries the session token
})
```

`cmd/dorgu-dashboard` is a thin wrapper over that same call, so the standalone
binary and the CLI subcommand cannot behave differently.

#### How the built SPA reaches the CLI binary

`go:embed` can only embed files that are in the module Go fetches, and the SPA is
built output that is not committed to `main`. So **every release tag points at a
commit that contains the built `internal/webui/dist`**, and the CLI depends on that
tag. `go install github.com/dorgu-ai/dorgu/cmd/dorgu@latest` then produces a
binary with a working dashboard and no Node toolchain on the installing machine.

`main` stays free of build output: the release commit is a child of `main` that is
reachable only through its tag. Cut one with:

```bash
make release-tag TAG=v0.1.0    # builds the SPA, commits dist on a detached HEAD, tags it
git push origin refs/tags/v0.1.0
```

The script refuses a dirty tree and a tag that already exists, and verifies the
embedded `index.html` before tagging. A pseudo-version of a branch commit (what
you get from `go get ...@<sha>`) has **no** SPA: the dashboard still starts, the
API works, and `/` explains that the UI is absent. Always depend on a release tag.

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

For the real server over invented data with no cluster and no envtest at all,
`hack/fixture-server` fills the store from `internal/fixtures` and serves the
production handlers and the embedded SPA. It has a scenario per empty state
(`full`, `warming`, `crds-missing`, `operator-idle`, `none`, `refused`) and is what
the Playwright suite and the CI screenshots drive:

```bash
make web fixture-server
./bin/fixture-server -scenario full -port 4173   # then open the URL it prints
```

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
  Deployments, Pods and Nodes. The HTTP layer reads the cache and never the API
  server, so twenty open tabs cost the API server nothing.
- **One poller, and only because the API demands it.** Node usage comes from
  `metrics.k8s.io`, which serves no `watch` verb, so it is read on an interval and
  the absence of an answer is a first-class result rather than a zero.
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
- **Every view is its own chunk.** Route-level code splitting, plus vendor chunks
  split by how often they change rather than by which view imports them first.
  Opening Apps no longer downloads the remediation diff renderer, and a change to
  a label re-downloads the app's own 17 KB rather than the whole bundle.

Every list is virtualised, so a cluster having a bad hour renders at the same
cost as an idle one.

### Layout

```
cmd/dorgu-dashboard   standalone binary and container entrypoint
pkg/dashboard         the package the dorgu CLI imports
internal/kube         kubeconfig loading, clients, CRD discovery
internal/informers    watches, feeding the cache
internal/metrics      the one poller: node usage from metrics-server
internal/store        the in-memory cache
internal/view         pure projections into the payloads the browser renders
internal/httpapi      REST, SSE and the embedded SPA
internal/sse          the event broker and its coalescer
internal/webui        go:embed of the built SPA
internal/workload     persona-to-Deployment matching and ownership detection
web/                  the Vite + React SPA
hack/devcluster       a real API server with fixtures, for local verification
hack/fixture-server   the real server over fixtures in memory, no cluster, for the e2e tests
pkg/report            the KPI computation shared with `dorgu report`
internal/fixtures     the fixture clusters: full, and the five empty states
```

`internal/workload` is a port of the operator's own `internal/workload`. Go
forbids importing another module's internal packages, and the dashboard has to
agree with the operator about which Deployment a persona describes and who owns
it, or it will show a different answer than `dorgu health` for the same cluster.
The logic is copied verbatim and pinned by tests that mirror the operator's. The
right fix is for the operator to promote that package to `pkg/`.

## Development

```bash
make check      # what the first CI job runs: gofmt, vet, race tests, web typecheck, lint, Vitest
make web        # build the SPA into internal/webui/dist
make build      # build the binary with whatever is in that directory
make web-dev    # Vite dev server on :5173, proxying /api to a dashboard on :7171
                # start one with: dorgu-dashboard --port 7171, then
                # DORGU_DASHBOARD_TOKEN=<the token it printed> make web-dev
make release-tag TAG=v0.1.0   # tag a commit that carries the built SPA (see above)
make cover      # test coverage per package
make e2e        # Playwright against fixture clusters: no Kubernetes needed
make e2e-screenshots   # a screenshot of every view in both themes, into web/screenshots
```

### Design system

[`DESIGN.md`](DESIGN.md) is the design system and the living reference: tokens,
type, components, states, motion and copy. It is enforced, not just written down:

- `npm run lint` rejects raw hex values and arbitrary or default-palette colour
  classes in `web/src`. Use a token, or add one with a reason.
- `npm test` runs the contrast check over every token pair the design uses, in both
  themes, reading the values from `src/styles.css`.
- `make web-check-no-external` fails if the built `index.html` or CSS could request
  anything from the network. Fonts (Inter and JetBrains Mono) are bundled.
- The Playwright suite runs axe in both themes on every view, panel and empty state,
  and fails on a serious or critical violation.

### Tests

- Go: `make test-race`. `pkg/report` carries the report's own tests, ported from the
  CLI.
- Frontend: Vitest and Testing Library for the components (gate strip, ticket chip,
  ownership badge, machine text block, guardrail meter, the five empty states, the
  review panel, the Overview).
- End to end: Playwright drives the real server and SPA over `hack/fixture-server`,
  one fixture cluster per scenario. On a fresh machine, `npx playwright install
  --with-deps chromium` in `web/` once, then `make e2e`.

`internal/webui/dist` holds a committed `.gitkeep` on purpose: without a file in
it, `go:embed all:dist` matches nothing and `go build` fails on a fresh clone. A
binary built without the frontend still runs, and serves an explanation at `/`
telling you to run `make web`.

`make web` prints the chunk sizes, which is worth glancing at. The app's own
entry chunk should stay small; the four view chunks are downloaded on navigation
and prefetched on hover. If a `vendor-*` chunk grows, a dependency did.

## Not in this release

- **The approve action.** The Proposals view shows the plan, the guardrail
  verdicts and the diff, and cannot apply any of it. That is a decision rather
  than an omission: the dashboard writes nothing to your cluster in this release,
  and the honest affordance is the CLI command that can act, not a disabled
  button that reads like something broken. Approve with
  `dorgu remediation approve`, which prints the same plan first.
- **User accounts.** There are none: one session token per run, for the person who
  started it. The dashboard acts as your kubeconfig and nothing else.
- **Telemetry.** Nothing here phones home.
- **A DorguEvents view.** The events are watched and cached and nothing renders
  them yet.

### What lifted the two gates that used to be here

Both of the views that were gated in the last release now render, and the reasons
they were gated were fixed upstream rather than worked around here:

- **Remediations** was gated because AI-planned remediations were not reliably
  applicable: clean-room run #4 measured nine of them and zero that could change
  a workload. Operator **v0.11.0** fixed that at the source, and a plan it cannot
  make appliable is now refused so the rule-based proposal takes over, recorded
  as `planSource: rule-based`. The same release added the optional
  `spec.steps[].safety` field this view renders. Operator **v0.11.1** is that
  release plus multi-arch images.
- **Cluster** was gated because cluster health could report impossible CPU
  figures. Operator **v0.11.0** stopped counting unscheduled pods and CLI
  **v0.12.0** stopped depending on the field. This view computes saturation
  itself, so it does not depend on which operator version is installed.

## Licence

Apache 2.0. See [LICENSE](./LICENSE).
