# kctl — Design

**Date:** 2026-09-14
**Status:** Approved, pending implementation plan

A Bubble Tea TUI for the kubectl operations its author performs daily: find a pod
and read its logs, switch among many contexts and namespaces, describe a pod that
will not start, and apply a manifest after seeing the diff and confirming the
target cluster.

---

## 1. Goals and non-goals

### Goals

- Replace the common `kubectl` loop with a keyboard-driven TUI.
- Make the current context and namespace impossible to miss.
- Never apply a manifest without showing a diff and naming the target cluster.
- Never print secret values that were not explicitly requested.

### Non-goals for v1

Named here so they do not creep in: exec into a pod, port-forward, delete any
resource, resource types beyond pods and secrets, edit-in-`$EDITOR`, Helm,
Kustomize, multi-cluster side-by-side views, metrics/top, a standalone events
browser, and a `kctl apply -f` shell entrypoint (apply is in-TUI only).

---

## 2. Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Cluster access | `client-go` directly | Real watch streams and typed objects; no dependency on a `kubectl` binary. |
| Apply entrypoint | In-TUI only | One binary, one surface. No shell entrypoint in v1. |
| Navigation | View stack (k9s-style) | Each view is an independent `tea.Model`: own file, own tests. Cheapest to extend. |
| Simultaneous views | None in v1 | Pins and split panes both deferred. See §2.1. |
| Pod data | Shared informer, cache-backed | Free reconnect and resync; cache serves filtering without re-fetching. |
| Secret data | On-demand `List`/`Get`, no cache | Secret material must not sit in a long-lived process. See §5. |
| Secret display | Redacted by default, explicit per-field reveal | Protects terminal scrollback and shoulder-surfing. |
| Apply safety | Dry-run diff always; typed confirm on protected clusters | See §7. |
| Protected-cluster detection | Cluster **server URL**, not context name | Context nicknames are renameable and often do not contain "prod". |
| Manifest source | File picker (rooted at cwd) plus raw path entry | Covers both discovery and paths outside cwd. |
| Namespace scope | Context's namespace, switchable, all-namespaces toggle | Many contexts pin a namespace; some work spans namespaces. |

### 2.1 Deferred, with reasons

- **Pinned log tails.** Would allow several tails alive at once. Requires a
  stream registry owning goroutine lifecycle, plus bounded-wait-on-quit so a
  half-dead TCP read cannot hang exit. Dropped from v1: one log view at a time
  means each view owns and cancels its own goroutine, and the registry is not
  needed at all. Revisit in v2.
- **Split detail pane.** Needs ~140 columns to be readable and a responsive
  degradation path. Describe is a pushed full-screen view instead.

---

## 3. Architecture

Three layers, one direction of dependency. The UI layer never imports
`client-go`.

```
┌─ ui (bubbletea) ────────────────────────────────┐
│  root model: stack + status bar + command bar   │
│  views: podlist, describe, logview, ctx, apply  │
└───────────────── depends on ────────────────────┘
                      ↓  (interfaces + plain structs)
┌─ core ──────────────────────────────────────────┐
│  role interfaces, domain types, selectors       │
└───────────────── implemented by ────────────────┘
                      ↓
┌─ kube (client-go) ──────────────────────────────┐
│  kubeconfig, clientsets, informer factory,      │
│  log streams, SSA dry-run diff + apply          │
└─────────────────────────────────────────────────┘
```

### 3.1 Role interfaces

Split by role rather than one large `Cluster` interface, so each view depends
only on what it uses and mocks stay small.

```go
// internal/core/ports.go
//go:generate mockgen -source=ports.go -destination=mocks/ports_mock.go -package=mocks

type ContextManager interface {
	Contexts() []ContextInfo
	Current() ContextInfo
	Use(ctx context.Context, name string) error
	Namespaces(ctx context.Context) ([]string, error)
}

type PodReader interface {
	Pods(ctx context.Context, sel Selector) ([]Pod, error)
	Pod(ctx context.Context, ns, name string) (*Pod, error)
	Events(ctx context.Context, ns, podName string) ([]Event, error)
	Subscribe(ctx context.Context, sel Selector) (<-chan struct{}, error)
	Logs(ctx context.Context, req LogRequest) (io.ReadCloser, error)
}

type SecretReader interface {
	Secrets(ctx context.Context, sel Selector) ([]SecretMeta, error)
	Secret(ctx context.Context, ns, name string) (*Secret, error)
}

type Applier interface {
	DryRun(ctx context.Context, docs []Manifest) ([]DryRunResult, error)
	Apply(ctx context.Context, docs []Manifest, force bool) ([]ApplyResult, error)
}
```

Every view is constructed with the narrow interface it needs. View tests inject
`mockgen`-generated mocks and need neither a live cluster nor an API server
binary. The apply pipeline is the one exception and is covered by `envtest` (§9).

### 3.2 Package layout

Deliberately flat to start. Split a package when it hurts, not before.

```
cmd/kctl/main.go
internal/
  core/        role interfaces, domain types, Selector, mocks/
  kube/        config.go client.go informers.go logs.go apply.go diff.go risk.go
  ui/
    app.go     root model, stack, status bar, command bar
    keys/      keymap: single source of truth, also feeds the help overlay
    views/     podlist/ describe/ logview/ secretlist/ ctxpicker/ apply/
    components/ table/ filter/ diffview/ confirm/ picker/
  theme/       lipgloss styles
```

Cohesion, not line count, decides when a file splits.

### 3.3 Logging

Bubble Tea owns stdout; a logger writing there corrupts the screen. `zap` writes
to `~/.cache/kctl/kctl.log`, configured once in `main.go` and threaded downward
with per-layer fields:

```go
logger = logger.With(zap.String("view", "podlist"), zap.String("context", ctxName))
```

The log file is forensics, not error reporting. Errors the user must see are
surfaced in the UI (§8).

---

## 4. Data flow

Two patterns, because pods and logs differ in kind.

### 4.1 Pods — cache is truth, events are pokes

The informer already maintains a correct, deduplicated cache. The view never
consumes watch events as data; it consumes them as "something changed, re-read".

```
informer ──add/update/delete──> poke(cap 1, non-blocking) ──> tea.Cmd ──> Update
                │                                                            │
                └──────────── lister.List(selector) <─────── re-read ────────┘
```

```go
// A poke already pending is as good as two.
func (s *podSource) poke() {
	select {
	case s.dirty <- struct{}{}:
	default:
	}
}
```

This removes an entire class of problems: no backpressure policy, no drop-oldest
rule, no event-ordering bugs, no unbounded queue. A burst of several hundred pod
updates during a rollout collapses into a single re-render.

The Bubble Tea side is a self-rearming command:

```go
func awaitPoke(dirty <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-dirty
		if !ok {
			return sourceClosedMsg{}
		}
		return dirtyMsg{}
	}
}
// Update: case dirtyMsg: m.rows = rebuild(lister.List(sel)); return m, awaitPoke(dirty)
```

Re-reading is a memory walk over a few hundred pods — cheaper than the render
it triggers.

### 4.2 Logs — the stream is truth

Log lines cannot be re-read; order and completeness matter.

```
GetLogs().Stream() ──> bufio.Scanner ──> chan string (buf 512) ──> tea.Cmd ──> ring buffer ──> viewport
```

- Ring buffer capped at 10k lines (configurable). Overflow drops oldest lines and
  marks the view header `truncated` — visible, never silent.
- The producer blocks when the channel fills. That is correct here: backpressure
  on a log tail simply means reading the socket more slowly.
- The consumer drains up to N lines per `Update`; one message per line would turn
  a chatty pod into thousands of renders per second.
- The view owns this goroutine and cancels it on Esc.

### 4.3 Secrets — neither

`List` when the view opens, `Get` on describe, discarded on pop. No cache, no
watch. Non-uniform with pods on purpose (§5).

### 4.4 Context and namespace switching

Sequenced, because half-switched state is a hard bug to find:

1. Cancel the current source's context; bounded wait (~2s), then abandon.
2. Status bar shows `connecting to <ctx>…` with a spinner. The stack keeps
   rendering stale rows, dimmed, rather than blanking.
3. Build the new clientset and informer factory; `WaitForCacheSync` under a
   timeout.
4. On success, swap the source, poke, un-dim. On failure, show an error and stay
   on the previous context — never leave the user nowhere.

The source carries a generation counter; pokes from a cancelled generation are
dropped in `Update`. This removes the late-event-from-old-context race.

### 4.5 Filter and sort

`/` filters the in-memory rows: case-insensitive substring on name for v1. Sort
keys: name (default), age, status, restarts. Both are pure functions over a row
slice and are unit-tested without a cluster.

---

## 5. Secrets

The informer cache would hold every secret's decoded `data` in process memory for
the lifetime of the program. Redaction protects the screen; it does nothing about
a core dump, a swapped page, or `/proc/<pid>/mem`. With the all-namespaces toggle
on, an informer would pull every secret in the cluster into one long-lived
process, and cluster-wide list/watch on secrets is a broad privilege to require.

Therefore secrets are never cached:

- The list view shows name, type, key count, and age — no values.
- Describe shows keys and byte counts: `DB_PASSWORD  32 bytes`.
- `x` reveals exactly one field's value, for as long as the view is open; Esc
  re-hides it.
- No reveal is written to the log file or any dump.
- The fetched object is dropped when the view pops.

---

## 6. Pod describe

Because "why will this not start" is a primary use case, describe includes events
in v1. `Pending`, `ImagePullBackOff`, and `CrashLoopBackOff` reasons live in
events, not in pod status.

Describe renders: metadata, node, QoS class, owner reference, container statuses
with restart reasons and exit codes, conditions, and volumes; then events for the
pod, newest first, fetched on demand with a field selector on `involvedObject`.
No events informer, no cache.

---

## 7. Apply / diff pipeline

Six stages. Stages 1–3 are pure and testable without a cluster.

```
file(s) ─1─> docs ─2─> normalize ─3─> SSA dry-run ─4─> diff ─5─> guard+confirm ─6─> SSA apply
```

**1. Parse.** Split on `---` via `yaml.NewYAMLOrJSONDecoder`, decode each document
into `*unstructured.Unstructured`. A document missing `apiVersion`, `kind`, or
`metadata.name` is rejected with its index: `doc 2: missing metadata.name`.

**2. Normalize.** Namespace resolution, in order: explicit
`metadata.namespace`, then the current namespace, then `default`. The resolved
namespace is displayed per document on the confirm screen — an implicit namespace
is how manifests land in the wrong place.

**3. Dry run.** Server-side apply with `FieldManager: "kctl"`, `DryRun: ["All"]`,
`Force: false`. The API server performs defaulting, validation, and admission,
and returns the object as it would exist.

**4. Diff.** Fetch the live object; a 404 means this is a creation, diffed against
empty. Serialize both to YAML and strip `managedFields`,
`metadata.resourceVersion`, `metadata.generation`, `metadata.creationTimestamp`,
and `status`. Render a unified diff (`hexops/gotextdiff`) in a scrollable
viewport, with a per-document summary line:
`deployment/api  ~3 fields  (2 changed, 1 added)`.

**5. Guard.** Risk level resolves from the cluster **server URL** in kubeconfig,
not the context nickname:

```yaml
# ~/.config/kctl/config.yaml
risk:
  - server: "https://api.zulu.*"
    level: protected
  - server: "https://api.alpha.*"
    level: protected
```

`normal` clusters confirm with `y`. `protected` clusters render the context name,
cluster server, namespace, and resource count, and require typing the context
name exactly. There is no default-yes and no enter-to-accept.

**6. Apply.** The same server-side apply without dry-run, sequentially in document
order. Each document reports its own result. On partial failure the screen states
exactly which documents applied and which did not.

**Conflicts.** Server-side apply returns `409` when another field manager owns a
field being set. The conflicting manager and fields are displayed, and forcing
requires arming an explicit toggle before re-confirming. Force is never silent:
a Flux-managed cluster is in the target set.

**Two dry-run caveats.** Mutating webhooks that do not declare
`sideEffects: None|NoneOnDryRun` cause the API server to reject the dry-run, so
some manifests cannot be previewed. And dry-run output can differ from reality
where other controllers default fields after admission. The diff is very good,
not perfect.

---

## 8. Error handling

### 8.1 Taxonomy

Four classes with four behaviors. Collapsing them into a single "show error" path
is what makes a TUI feel broken.

| Class | Example | Behavior |
|---|---|---|
| Fatal | kubeconfig unreadable, no contexts | Fail before `tea.NewProgram`; plain stderr message, exit 1. |
| View-scoped | `list pods` returns 403 | View renders an error state in place with the reason and `r` to retry; the stack stays navigable. |
| Transient stream | watch dropped, API server restart | Auto-retry with exponential backoff capped at 30s; status bar shows `⟳ reconnecting`. No modal. |
| User input | malformed YAML, bad namespace | Inline, next to the input that caused it — not a toast. |

### 8.2 Conventions

```go
func (c *cluster) Pods(ctx context.Context, sel core.Selector) (pods []core.Pod, err error) {
	logger := c.logger.With(zap.String("method", "Pods"), zap.String("namespace", sel.Namespace))

	list, err := c.podLister.Pods(sel.Namespace).List(sel.Label)
	if err != nil {
		err = fmt.Errorf("error-listing-pods :%w", err)
		logger.Error("error-listing-pods", zap.Error(err))
		return pods, err
	}

	pods = toDomainPods(list)
	return pods, err
}
```

Errors are handled on their own line, wrapped with `-`-joined contextual
messages, logged through a logger threaded from `main.go` with per-layer fields,
and named returns are returned by variable.

### 8.3 Terminal restoration

A panic in `Update` or in a `tea.Cmd` leaves the terminal in raw mode, with no
echo and no line discipline. Bubble Tea's panic catching stays enabled, and
`main.go` defers a terminal restore that re-prints the panic after the program
returns.

---

## 9. Testing

**Pure functions — table-driven, no cluster.** Filter, sort, multi-document
splitting, namespace resolution, diff field-stripping, and risk-level resolution
from a server URL. This is the bulk of the logic and runs in microseconds.

**Views — generated mocks plus `teatest`.** `mockgen` off the role interfaces per
project standard; `charmbracelet/x/exp/teatest` drives key sequences and asserts
against golden files. Coverage: filter narrows rows, Enter pushes describe, Esc
pops, a failing mock renders the error state, redaction holds until `x`.

**kube layer — `client-go` fake clientset** for list, get, and watch, including
the fake watcher driving the poke path.

**Apply pipeline — `envtest`.** The fake clientset does not implement
server-side apply correctly, so testing SSA dry-run, field-manager conflicts, and
the 409 force path against it would give false confidence. `envtest` runs a real
`kube-apiserver` and `etcd` locally (~3s startup) and is used for the apply tests
only. Apply is the one feature that can damage a cluster; it gets real coverage.

**Not tested:** lipgloss styling and exact ANSI output. Golden files assert
structure, or every theme tweak breaks the suite.

---

## 10. Build order

Shipped as usable slices rather than architectural layers. Each slice is
independently useful.

### v0.1 — find the pod, read its logs

| # | Milestone |
|---|---|
| 0 | Skeleton: `main.go`, file logger, terminal restore, `q` quits cleanly |
| 1 | kubeconfig load, context picker, `:ns` switch, status bar shows context and namespace |
| 2 | Pod list: informer, poke bridge, live table |
| 3 | `/` filter, sort keys |
| 4 | View stack: Enter pushes, Esc pops |
| 5 | Log view: stream, ring buffer, follow toggle, Esc closes and cancels |

At this point kctl replaces `kubectl config use-context`, `get pods`, and
`logs -f`.

### v0.2 — why will it not start

| # | Milestone |
|---|---|
| 6 | Describe pod: container states, restart reasons, conditions |
| 7 | Events for the selected pod, newest first |

### v0.3 — apply with diff

| # | Milestone |
|---|---|
| 8 | File picker and path entry, multi-document parse, namespace resolution |
| 9 | Server-side dry-run, diff rendering |
| 10 | Risk guard, typed confirm, real apply, 409 conflict path |

### v0.4 — secrets

| # | Milestone |
|---|---|
| 11 | On-demand list, describe, redacted reveal |

### v1.0 — polish

| # | Milestone |
|---|---|
| 12 | Help overlay, `~/.config/kctl/config.yaml`, README |

---

## 11. Known weaknesses

Carried forward deliberately rather than solved:

- **Domain types drift toward copies of `corev1.Pod`.** Describe wants many
  fields, and each one costs an edit in three places. Accepted; revisit by
  embedding the raw object if the churn becomes real.
- **The role-interface seam's only real payoff is test speed.** There will never
  be a second implementation besides the mock. That is still worth it, but it is
  one benefit, not three.
- **Risk configuration is opt-in.** A cluster absent from `risk:` is treated as
  `normal`. A first-run prompt to classify unknown clusters is a v2 candidate.
- **Dry-run is not reality** (§7). Some manifests cannot be previewed at all.
- **Log scrollback is lossy** beyond the ring buffer cap.
