# kctl

A terminal UI for the `kubectl` work you do every day: find a pod, read its logs,
switch contexts and namespaces without retyping them.

```
 ctx kind-dev  ns all namespaces

 NAMESPACE           NAME                                      READY  STATUS    RESTARTS  AGE
 kube-system         coredns-559f6c778d-l2wnq                  1/1    Running   0         6h
 kube-system         etcd-dev-control-plane                    1/1    Running   0         6h
 kube-system         kube-apiserver-dev-control-plane          1/1    Running   0         6h
 local-path-storage  local-path-provisioner-75f7fc7dc5-fxlcj   1/1    Running   0         6h
 default             nginx-2-77cfbc5d59-fwvsh                  1/1    Running   0         5h

 11/11 pods  ·  sort:name  ·  /:filter  s:sort  enter:select  esc:back
 ::command  q:quit
```

## Install

Requires Go 1.26.

```bash
go build -o kctl ./cmd/kctl
./kctl
```

kctl opens on whatever context your kubeconfig currently points at, scoped to that
context's namespace. It reads `KUBECONFIG` if set, otherwise `~/.kube/config`.

## Keys

| Key | Action |
| --- | --- |
| `↑` `↓` | Move the selection |
| `/` | Filter the list by name; `esc` clears it |
| `s` | Cycle the sort order: name → status → restarts → age |
| `enter` | Tail the selected pod's logs |
| `d` | Describe the selected pod, with its events |
| `a` | Apply a manifest, after showing the diff |
| `f` | In the log view: pause or resume following |
| `r` | Retry after a failed read |
| `c` | Open the context picker |
| `:` | Open the command bar |
| `esc` | Go back one view |
| `q` / `ctrl+c` | Quit |

While a filter or the command bar has focus, letter keys type into it — `q` does not
quit mid-word.

## Commands

| Command | Effect |
| --- | --- |
| `:ns <name>` | Scope the pod list to one namespace |
| `:ns all` | Show every namespace, adding a NAMESPACE column |
| `:ctx` | Open the context picker |

## Describe

`d` shows why a pod is unhappy: per-container state with the reason and exit code
behind it, conditions, and the pod's events newest first with warnings highlighted.

Events are fetched live rather than watched — the reason a pod will not start is
exactly the thing that must not be served stale. If your RBAC grants pods but denies
events, the detail still renders and the events section is simply empty.

## Apply

`a` asks for a manifest path, then shows what the **server** says would change: a
real server-side apply with `DryRun=All`, so the diff reflects defaulting, validation
and admission rather than a client-side guess. Nothing is written until you confirm.

Three things block the write:

- **A field-ownership conflict.** The dry run reports which manager owns which
  fields, so you see `conflict with flux over .data.tier` before deciding rather
  than after a half-applied set. Forcing is a separate armed toggle (`F`).
- **A document that failed its dry run.**
- **A protected cluster**, until you type the context name exactly.

### Protected clusters

Risk is keyed on the cluster's **API server URL**, never the context name. Context
names are local nicknames that get renamed, and a production cluster's context is
frequently not called anything resembling "prod".

```yaml
# ~/.config/kctl/config.yaml
risk:
  - server: "https://zulu-apiserver-*"
    level: protected
  - server: "https://apiserver-alpha-*"
    level: protected
```

A cluster matching no rule is normal. Failing open is deliberate: the alternative
trains you to type a context name for every unremarkable apply, which is how a
confirmation becomes muscle memory and stops being read.

## What it does

**Pods update themselves.** kctl watches the cluster through a client-go informer
rather than polling, so the list reflects rollouts and restarts as they happen.
Watch events are only ever used as a signal to re-read the informer's cache, never
as data — a burst of several hundred updates during a rollout collapses into a
single redraw.

**Status is the real reason, not the phase.** A pod stuck on `ImagePullBackOff`,
`CrashLoopBackOff`, `Init:0/2`, `Evicted` or `Terminating` says so. Showing the raw
pod phase would render most broken pods as `Running` or `Pending`, which defeats
the point.

**Switching contexts never edits your kubeconfig.** The active context is process
local, so kctl cannot change what your other terminals are pointed at. A switch to
an unreachable cluster leaves you where you were with an error, rather than
stranding you on a dead context with an empty list.

**Log tailing is bounded.** Lines land in a ring buffer capped at 10k. Older lines
are dropped and the header says `truncated`, so loss is visible rather than silent.
Leaving the log view closes the stream — browsing twenty pods does not leave twenty
connections open.

## Not built yet

- Secrets browsing, with values redacted until explicitly revealed. The design is
  deliberate about this: secrets are never cached, so secret material does not sit
  in a long-lived process.

The design is in
[`docs/superpowers/specs/2026-09-14-kctl-design.md`](docs/superpowers/specs/2026-09-14-kctl-design.md).

Also absent: an in-app help overlay, and a file picker for apply (paths are typed).

## Logs

kctl is a full-screen program, so diagnostics go to a file rather than the terminal:

```
~/Library/Caches/kctl/kctl.log      # macOS
~/.cache/kctl/kctl.log              # Linux
```

That file is for forensics. Anything you need to act on is shown in the UI.

## Layout

```
cmd/kctl           entry point, logger, panic guard
internal/core      domain types and role interfaces — imports only the stdlib
internal/kube      client-go: kubeconfig, informers, log streams
internal/ui        Bubble Tea root model, view stack, status bar, command bar
internal/ui/views  podlist, logview, describe, apply, ctxpicker
```

`internal/core` is the seam. It imports no Kubernetes, Bubble Tea or logging
packages, which is what lets every view be tested against a generated mock with no
cluster and no API server binary. A test enforces that boundary rather than trusting
it.

## Tests

```bash
go test ./... -race
```

143 tests across 16 packages. The cluster-facing code is tested against client-go's
fake clientset, and the views through `teatest`, so the suite needs no cluster.
