# How kctl is put together

Orientation for working on this codebase. The [README](README.md) covers what it
does; this covers where things live and why.

## The one idea

Three layers, dependencies pointing one way only:

```
internal/ui      Bubble Tea. Knows nothing about Kubernetes.
      ↓ depends on
internal/core    Interfaces and plain structs. Imports only the standard library.
      ↑ implemented by
internal/kube    client-go. Knows nothing about the terminal.
```

`internal/core` is the hinge. It imports no Kubernetes, no Bubble Tea, no logging
— a test enforces that (`internal/core/imports_test.go`, and it does fail: drop a
`charm.land/bubbletea/v2` import in there and watch).

That boundary is what lets every view be tested against a generated mock with no
cluster and no API server. It is worth protecting; if you find yourself wanting a
`corev1.Pod` in a view, convert it in `internal/kube` instead.

## Reading order

If you want to understand the whole thing, read in this order — about 800 lines:

1. `internal/core/types.go` — the domain: `Pod`, `PodDetail`, `Event`, `Selector`
2. `internal/core/ports.go` — the four interfaces the UI depends on
3. `internal/kube/pods.go` — the informer and the poke pattern (the core mechanism)
4. `internal/ui/app.go` — the root model: routing, the stack, message handling
5. `internal/ui/views/podlist/podlist.go` — a representative view

## The data-flow mechanism worth understanding

Pods are not polled and watch events are never used as data. The informer keeps a
cache; a watch event only pokes subscribers to re-read it.

```
informer ──add/update/delete──> poke(cap 1, non-blocking) ──> tea.Cmd ──> Update
                │                                                          │
                └────────── lister.List(selector) <──── re-read ───────────┘
```

```go
// internal/kube/pods.go — a poke already pending is as good as two
func (s *PodSource) poke() {
	for _, ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
```

Because the channel holds one token and sends never block, a burst of four hundred
pod updates during a rollout collapses into a single re-render. There is no
backpressure policy, no drop-oldest rule, and no event ordering to get wrong —
that whole class of bug is absent by construction rather than handled.

Logs are the opposite: the stream *is* the truth, order matters, lines cannot be
re-read. So `logview` keeps a capped ring buffer and drains the channel in batches
(`internal/ui/views/logview/`). Two different problems, two different shapes.

## How a keystroke travels

`internal/ui/app.go` is the router. Order matters and is deliberate:

1. **`ctrl+c`** — always, before anything else. There must be a way out.
2. **Command bar**, if open — it owns the keyboard.
3. **`:`** opens the command bar.
4. **A view's focused text input** — if the top view reports `InputFocused()`,
   global single-letter bindings are skipped. This is what lets you type a
   namespace containing `q` without quitting.
5. **Globals** — quit, apply, context picker, back.
6. **Everything else** falls through to the top view.

A view never pops itself. It emits a message (`podlist.DescribeMsg`,
`apply.CancelMsg`) and the root decides. That keeps navigation in one place.

## Adding a view

Five steps, following `describe` as the template:

1. **Domain types** in `internal/core/types.go` — plain structs, no k8s imports.
2. **A port** in `internal/core/ports.go` if you need new cluster data. Keep it
   narrow: `PodDescriber` is two methods, not bolted onto `PodReader`. Then
   `go generate ./internal/core/` to regenerate mocks.
3. **The kube implementation** in `internal/kube/` — convert to domain types there.
4. **The view** in `internal/ui/views/yours/` — a `tea.Model` returning `tea.View`.
5. **Wire it** in `app.go`: a key binding in `internal/ui/keys/keys.go`, a message
   the root handles, and a `pushSized(...)` call.

`pushSized`, not `stack.Push`. See the traps below.

## Traps this codebase has already hit

Each of these cost real debugging time. They are not hypothetical.

**Bubble Tea v2 is not v1.** `View()` returns `tea.View`, not `string`. The alt
screen is a field on that view (`v.AltScreen = true`), set by the root model only —
`tea.WithAltScreen()` no longer exists. `tea.KeyMsg` is now `tea.KeyPressMsg`.

**`bubbles/v2` tables render zero rows without `WithWidth`.** `WithHeight` alone
gives you a header and nothing else. The table looks configured and silently shows
no data.

**A pushed view gets no `WindowSizeMsg`.** Bubble Tea sends it at startup and on
resize, so a view pushed later never learns its size, its viewport stays zero-height,
and it renders nothing — which looks exactly like the keypress being ignored. Always
push via `pushSized`.

**Paste is not key presses.** It arrives as `tea.PasteMsg`. A view that only routes
`KeyPressMsg` silently drops pasted text.

**Anything can write to your terminal.** client-go runs kubeconfig exec credential
plugins as *child processes* and hands them `os.Stderr`; klog writes there too. An
OIDC helper failing to authenticate prints straight over the UI. `logging.RedirectStderr`
moves fd 2 to the log file for the lifetime of the program — reassigning the
`os.Stderr` variable is not enough, because a child inherits the descriptor.

**`teatest.WaitFor` drains the output stream.** A test that waits three times sees
nothing on the third. If you need to assert "row appears, then disappears, then
returns", test at the model level instead.

## Testing

```bash
go test ./... -race
```

No cluster needed. Cluster code runs against client-go's fake clientset; views run
against `teatest` and generated mocks.

**The bar for a test here:** it must be able to fail. Several tests in this repo's
history passed whether or not the feature worked — asserting a substring that was
present either way. When you write a guard, break the thing it guards and watch it
go red. If it does not, the test is decoration.

Model-level tests also missed five real bugs that only appear in an assembled
binary. For UI work, drive the real thing:

```bash
tmux new-session -d -s t -x 120 -y 30 './kctl' && sleep 3
tmux send-keys -t t 'd' && sleep 1
tmux capture-pane -t t -p
```

`expect` will not work — it cannot reconstruct the v2 alt-screen renderer and will
report a blank screen for a working program.

## Where the reasoning lives

`git log` is the design record; commit messages explain why, not just what. For the
bigger decisions:

- `docs/superpowers/specs/2026-09-14-kctl-design.md` — the original design, including
  deliberately rejected alternatives and known weaknesses
- `docs/superpowers/plans/` — the implementation plan, now partly superseded by what
  the code actually does

Both predate the Bubble Tea v2 migration in places. The code is the truth.

## Known rough edges

- `internal/ui/app.go` is 418 lines and is the natural next thing to split — routing,
  session handling, and view wiring are three jobs in one file.
- `internal/ui/views/apply/apply.go` at 592 lines carries a four-stage state machine
  that would read better as explicit states.
- The apply flow has no file browser, only tab completion.
- Secrets are designed (spec §5) but unbuilt. The design matters: never cached, so
  secret material does not sit in a long-lived process.
