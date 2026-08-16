# fluentfp

**Fluent functional programming for Go.**

Type-safe collection chains, composable resilience (retry, circuit breaker), typed HTTP handlers, and optional/result types — all on standard Go, no framework required.

See [pkg.go.dev](https://pkg.go.dev/github.com/binaryphile/fluentfp) for API docs and the **[showcase](docs/showcase.md)** for 24 before/after rewrites from real GitHub projects.

Zero reflection. Zero global state. Zero build tags.

## Quick Start

Requires Go 1.26+.

```bash
go get github.com/binaryphile/fluentfp
```

```go {ignore}
import "github.com/binaryphile/fluentfp/slice"

// Before: scaffolding around one line of intent
var names []string                         // state
for _, u := range users {                  // iteration
    if u.IsActive() {                      // predicate
        names = append(names, u.Name())    // accumulation
    }
}

// After: intent only
names := slice.From(users).KeepIf(User.IsActive).ToString(User.Name)
```

That's a **fluent chain** — each step returns a value you can call the next method on, so the whole pipeline reads as a single expression: filter, then transform.

### Method Expressions

Go lets you reference a method by its type name, creating a function value where the receiver becomes the first argument:

```go {ignore}
func (u User) IsActive() bool  // method
func(User) bool                // method expression: User.IsActive
```

`KeepIf` expects `func(T) bool` — `User.IsActive` is exactly that:

```go {ignore}
names := slice.From(users).KeepIf(User.IsActive).ToString(User.Name)
```

Without method expressions, every predicate needs a wrapper: `func(u User) bool { return u.IsActive() }`.

For `[]*User` slices, the method expression is `(*User).IsActive`.

See [naming patterns](naming-in-hof.md) for when to use method expressions vs named functions vs closures.

### Beyond Collections

fluentfp isn't just `slice`. Here's the same library applied to HTTP handlers, resilience, and request plumbing:

```go {ignore}
// HTTP handler returns a value — no ResponseWriter mutation
handleGetUser := func(r *http.Request) rslt.Result[web.Response] {
    return rslt.Map(
        option.New(store.Get(r.PathValue("id"))).OkOr(web.NotFound("user not found")),
        web.OK[User],
    )
}
mux.HandleFunc("GET /users/{id}", web.Adapt(handleGetUser))
```

```go {ignore}
// Chainable resilience — retry, then circuit breaker
breaker := wrap.NewBreaker(wrap.BreakerConfig{
    ResetTimeout: 10 * time.Second,
    ReadyToTrip:  wrap.ConsecutiveFailures(3),
})
safeFetch := wrap.Func(fetchFromAPI).
    Retry(3, wrap.ExpBackoff(time.Second), nil).
    Breaker(breaker)
resp, err := safeFetch(ctx, url)  // returns wrap.ErrCircuitOpen when tripped
```

```go {ignore}
// Typed context values — no sentinel keys, no type assertions
ctx = ctxval.With(ctx, RequestID("req-123"))
reqID := ctxval.Lookup[RequestID](ctx).Or("unknown")
```

See the [orders example](examples/orders/) for all of these composing in a single runnable service.

## What It Looks Like

The **[showcase](docs/showcase.md)** has 22 more rewrites across slice operations, lazy streams, concurrency, algorithm decomposition, function decoration, option chaining, and enterprise patterns (fold, middleware-as-data, saga).

### Conditional Struct Fields

Go struct literals let you build and return a value in one statement — fluentfp keeps it that way when fields are conditional.

<table>
<tr><th>Before</th><th>After</th></tr>
<tr><td><pre><code class="language-go">var level string
if overdue {
    level = "critical"
} else {
    level = "info"
}
var icon string
if overdue {
    icon = "!"
} else {
    icon = "✓"
}
return Alert{
    Message: msg,
    Level:   level,
    Icon:    icon,
}
</code></pre></td><td><pre><code class="language-go">return Alert{
    Message: msg,
    Level:   option.When(overdue, "critical").Or("info"),
    Icon:    option.When(overdue, "!").Or("✓"),
}
</code></pre></td></tr>
</table>

Go has no inline conditional expression. `option.When` fills that gap — each field resolves in place, so the struct literal stays a single statement. *From [hashicorp/consul](https://github.com/hashicorp/consul/blob/554b4ba24f86/agent/agent.go#L2482-L2530).*

### Bounded Concurrent Requests

Fetch weather for a list of cities with at most 10 simultaneous goroutines.

<table>
<tr><th>Before — errgroup (21 lines)</th><th>After (1 line)</th></tr>
<tr><td><pre><code class="language-go">func Cities(ctx context.Context, cities ...string) ([]*Info, error) {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(10)
    res := make([]*Info, len(cities))
    for i, city := range cities {
        g.Go(func() error {
            info, err := City(ctx, city)
            if err != nil {
                return err
            }
            res[i] = info
            return nil
        })
    }
    if err := g.Wait(); err != nil {
        return nil, err
    }
    return res, nil
}
</code></pre></td><td><pre><code class="language-go">func Cities(ctx context.Context, cities ...string) ([]*Info, error) {
    return slice.FanOutAll(ctx, 10, cities, City)
}
</code></pre></td></tr>
</table>

`FanOutAll` is all-or-nothing: on first error it cancels remaining work and returns that error. `City` passes directly — no wrapper needed. Panics in callbacks are recovered as `*rslt.PanicError` with stack trace.

When you need per-item outcomes instead of all-or-nothing, use `FanOut`:

```go {ignore}
results := slice.FanOut(ctx, 10, cities, City)
infos, errs := rslt.Partition(results)  // gather successes and failures separately
```

*From the [errgroup pattern](https://encore.dev/blog/advanced-go-concurrency).*

## Why fluentfp

**Type-safe end-to-end.** [go-linq](https://github.com/ahmetb/go-linq) gives you `[]any` back — type-assert it and hope you got the type right. [lo](https://github.com/samber/lo) requires `func(T, int)` callbacks, so every stdlib function needs a wrapper to discard the unused index. fluentfp uses generics throughout: `Mapper[T]` is `[]T` with methods. If it compiles, you avoid a class of type-assertion, index, and callback-shape mistakes.

**Fewer places for bugs to hide.** No index means no off-by-one in a predicate. No loop variable means no shadowing. No accumulator means no forgetting to initialize one. These are the loop-scaffolding bug classes that code review catches regularly — fluentfp removes the scaffolding where they live.

**Works with Go, not against it.** Mappers are slices — callers of your functions don't need to import fluentfp. Options use comma-ok (`.Get() (T, bool)`), the same pattern as map lookups and type assertions. `either.Fold` gives you exhaustive dispatch the compiler enforces — miss a branch and it doesn't compile. `must.BeNil` makes invariant enforcement explicit. Mutation, channels, and hot paths stay as loops.

## Interchangeable Types

`Mapper[T]` is defined as `type Mapper[T any] []T` — a [defined type](https://go.dev/ref/spec#Type_definitions), not a wrapper. `[]T` and `Mapper[T]` convert implicitly in either direction, so you choose how much to expose:

```go {ignore}
// Public API — hide the dependency. Callers never see fluentfp types.
func ActiveNames(users []User) []string {
    return slice.From(users).KeepIf(User.IsActive).ToString(User.Name)
}

// Internal — embrace it. Accepting Mapper saves From() calls across a chain of helpers.
func transform(users slice.Mapper[User]) slice.Mapper[User] {
    return users.KeepIf(User.IsActive).Transform(User.Normalize)
}
```

The public pattern keeps fluentfp as an implementation detail — callers don't import it, and its types don't appear in intellisense. Internal code can pass Mappers between helpers to avoid repeated `From()` wrapping.

## Performance

A single chain step (filter OR map) matches a hand-written loop — `slice.From` is a zero-cost type conversion, and each operation pre-allocates with `make([]T, 0, len(input))`.

Multi-step chains pay one allocation per stage. A chain that filters then maps makes two passes and two allocations where a hand-written loop can fuse them into one. In [benchmarks](methodology.md#benchmark-results) (1000 elements), a two-step chain runs ~2.5× slower than the fused equivalent.

If you're counting nanoseconds in a hot path, fuse it in a loop. Most loops aren't hot paths — they're scaffolding that fluentfp eliminates.

## Measurable Impact

| Codebase Type   | Code Reduction | Complexity Reduction |
| --------------- | -------------- | -------------------- |
| Mixed (example) | 12%            | 26%                  |
| Pure pipeline   | 47%            | 95%                  |

*Individual loops see up to 6x line reduction. Codebase-wide averages are lower because not every line is a loop. Complexity measured via `scc`. See [methodology](methodology.md#f-code-metrics-tool-scc).*

## When to Use Loops

fluentfp replaces mechanical loops — iteration scaffolding around a predicate and a transform. It doesn't try to replace loops that do structural work:

```go {ignore}
// Mutation in place — fluentfp returns new slices, but elements are shared (shallow copy)
for i := range items {
    if items[i].ID == target {
        items[i].Status = "done"
        break
    }
}

// Channel consumption — direct range is simplest for straightforward use
for msg := range ch {
    handle(msg)
}

// Bridge to fluentfp when you want operators on a channel
seq.FromChannel(ctx, ch).KeepIf(valid).Take(10).Each(handle)

// Complex control flow — early return, labeled break
for _, item := range items {
    if item.IsTerminal() {
        return item.Result()
    }
}

// Hot paths — fuse filter+map into one pass when nanoseconds matter
out := make([]R, 0, len(input))
for _, v := range input {
    if keep(v) {
        out = append(out, transform(v))
    }
}
```

## Packages

Packages are independent — import one or all.

| Package             | Purpose                          | Key Functions                                  |
| ------------------- | -------------------------------- | ---------------------------------------------- |
| [slice](slice/)     | Collection transforms            | `KeepIf`, `RemoveIf`, `Fold`, `TryFold`, `FanOutAll` |
| [kv](kv/)           | Map transforms                   | `KeepIf`, `MapValues`, `Map`, `Values`         |
| [seq](seq/)         | Fluent iter.Seq chains           | `From`, `KeepIf`, `Take`, `Collect`            |
| [stream](stream/)   | Lazy memoized sequences          | `Generate`, `Unfold`, `Take`, `Collect`        |
| [option](option/)   | Optional values + conditionals   | `Of`, `When`, `Or`, `NonZero`, `Env`           |
| [either](either/)   | Sum types                        | `Left`, `Right`, `Fold`, `Transform`, `FlatMap`  |
| [rslt](rslt/)       | Typed error handling             | `Ok`, `Err`, `CollectAll`, `Partition`   |
| [must](must/)       | Invariant enforcement            | `Get`, `BeNil`, `Of`                           |
| [hof](hof/)         | Higher-order functions              | `Pipe`, `Bind`, `Cross`, `Eq`, `NewDebouncer`    |
| [wrap](wrap/)           | Resilience decorators              | `Func`, `Retry`, `Breaker`, `MapError`, `OnError` |
| [memctl](memctl/)   | Memory-aware controller          | `Watch`, `MemInfo`, `Headroom`                 |
| [ctxval](ctxval/)   | Typed context values             | `With`, `From`, `NewKey`                       |
| [web](web/)         | Typed HTTP handlers              | `Adapt`, `DecodeJSON`, `Steps`                 |
| [memo](memo/)       | Memoization                      | `Of`, `Fn`, `FnErr`, `NewLRU`                  |
| [heap](heap/)       | Persistent priority queue        | `New`, `Insert`, `Pop`, `Collect`              |
| [pair](tuple/pair/) | Zip slices                       | `Zip`, `ZipWith`                               |
| [combo](combo/)     | Combinatorial constructions (eager + lazy Seq) | `CartesianProduct`, `Combinations`, `SeqPermutations` |
| [lof](lof/)         | Lower-order function wrappers    | `Len`, `Println`, `Identity`, `Inc`            |

## Package Highlights

**[wrap](wrap/)** — chainable resilience decorators:

```go {ignore}
// Retry transient errors, then circuit-break the dependency
breaker := wrap.NewBreaker(wrap.BreakerConfig{ResetTimeout: 30 * time.Second})

safeFetch := wrap.Func(fetchData).
    Retry(3, wrap.ExpBackoff(100*time.Millisecond), isTransient).
    Breaker(breaker)
```

**[web](web/)** — typed HTTP handlers on net/http:

```go {ignore}
// Handlers return Result[Response] — no ResponseWriter, no manual status codes
var createUser web.Handler = func(r *http.Request) rslt.Result[web.Response] {
    req, err := web.DecodeJSON[CreateReq](r)
    return rslt.Map(rslt.Of(req, err), createAndRespond)
}

// Adapt bridges to http.HandlerFunc; WithErrorMapper translates domain errors
endpoint := web.Adapt(createUser, web.WithErrorMapper(domainToHTTP))
mux.HandleFunc("POST /users", endpoint)
```

**[ctxval](ctxval/)** — typed context values without type assertions:

```go {ignore}
type RequestID string
ctx = ctxval.With(ctx, RequestID("abc-123"))
reqID := ctxval.Lookup[RequestID](ctx)  // Option[RequestID]
```

**[rslt](rslt/)** — typed error handling as values:

```go {ignore}
r := rslt.Of(strconv.Atoi(input))  // wrap (int, error) → Result[int]
port := r.Or(8080)                   // value or default
```

**[seq](seq/)** — fluent chains on Go's `iter.Seq`:

```go {ignore}
active := seq.FromIter(maps.Keys(configs)).KeepIf(isActive).Collect()
```

**[stream](stream/)** — lazy memoized sequences:

```go {ignore}
naturals := stream.Generate(0, lof.Inc)
first10Squares := stream.Map(naturals, square).Take(10).Collect()
```

## Capability Map

| If you need to... | Use | Package |
| --- | --- | --- |
| Filter, map, or fold a slice | `slice.From(s).KeepIf(f).ToString(g)` | slice |
| Fold with early exit on error | `slice.TryFold(events, state, fn)` | slice |
| Conditionally filter in a chain | `slice.From(s).KeepIfWhen(cond, f)` | slice |
| Run work concurrently with a limit | `slice.FanOutAll(ctx, 10, items, fn)` | slice |
| Retry on failure with backoff | `wrap.Func(fn).Retry(3, backoff, pred)` | wrap |
| Circuit-break an unhealthy dependency | `wrap.Func(fn).Breaker(breaker)` | wrap |
| Transform errors in a decorator chain | `wrap.Func(fn).MapError(mapper)` | wrap |
| Debounce rapid calls | `hof.NewDebouncer(wait, fn)` | hof |
| Represent optional values | `option.Of(v)`, `option.NonZero(v)`, `option.Env("KEY")` | option |
| Inline conditional (no ternary in Go) | `option.When(cond, val).Or(fallback)` | option |
| Handle (value, error) as a single value | `rslt.Of(strconv.Atoi(s))` | rslt |
| Collect per-item outcomes from FanOut | `rslt.Partition(results)` | rslt |
| Exhaustive two-branch dispatch | `either.Fold(e, onLeft, onRight)` | either |
| Panic on invariant violation | `must.Get(fn())`, `must.BeNil(err)` | must |
| Store typed values in context.Context | `ctxval.With(ctx, val)` / `ctxval.Lookup[T](ctx)` | ctxval |
| Build typed HTTP handlers on net/http | `web.Adapt(handler, web.WithErrorMapper(m))` | web |
| Decode JSON request bodies | `web.DecodeJSON[T](r)` | web |
| Extract path parameters as Option | `web.PathParam(req, "id")` | web |
| Partially apply context to a call | `rslt.LiftCtx(ctx, fn)` | rslt |
| Apply fallible fn to Option (absent=ok, invalid=err) | `option.MapResult(opt, fn)` | option |
| Lazy iterate with memoization | `stream.Generate(seed, fn).Take(10).Collect()` | stream |
| Lazy iterate without memoization | `seq.From(s).KeepIf(f).Take(10).Collect()` | seq |
| Memoize a function | `memo.From(fn)` or `memo.Fn(cache, fn)` | memo |
| Work with maps functionally | `kv.Keys(m)`, `kv.MapValues(m, fn)` | kv |
| Generate combinations/permutations | `combo.Combinations(items, k)` | combo |
| Lazy combinatorial generation | `combo.SeqPermutations(items).Take(10)` | combo |
| Use a persistent priority queue | `heap.New(cmp).Insert(v)` | heap |
| Zip two slices into pairs | `pair.Zip(as, bs)` or `pair.ZipWith(as, bs, fn)` | pair |

## Examples

| Example | Packages | Description |
|---------|----------|-------------|
| [orders](examples/orders/) | web, ctxval, option, rslt, slice | Curl-testable order processing service — full cross-package composition demo |
| [resilient_client](examples/resilient_client.go) | call | Circuit breaker + retry + error classification in 20 lines |
| [middleware_stack](examples/middleware_stack.go) | web, call, ctxval, option, rslt | HTTP middleware stack with breaker, request ID, and error mapping |

Run with `go run ./examples/orders/` or `go run examples/<file>.go`.

## Further Reading

- [Full Analysis](analysis.md) - Technical deep-dive with examples
- [Methodology](methodology.md) - How claims were measured
- [Nil Safety](nil-safety.md) - The billion-dollar mistake and Go
- [Naming Functions](naming-in-hof.md) - Function naming patterns for HOF use
- [Library Comparison](comparison.md) - How fluentfp compares to alternatives
- [Real-World Showcase](docs/showcase.md) - Before/after rewrites from GitHub projects

See [CHANGELOG](CHANGELOG.md) for version history.

## License

fluentfp is licensed under the MIT License. See [LICENSE](LICENSE) for more details.
