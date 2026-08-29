# Contributing

TinyCloud is a small project with one maintainer. That shapes what is useful to
send and what will sit unreviewed, so this document is mostly about the former.

## The contribution that matters most right now

**Write the second provider.**

Core reads every substrate through the [`/v0` contract](provider-contract-v0.yaml),
and that contract has met exactly one substrate: Kubernetes. Until a second
author implements it against a cloud the maintainer does not use, nobody knows
which parts of it are genuinely neutral and which are Kubernetes-shaped wearing
a neutral name.

This has already happened once. The first outside substrate found the metric
capability shaped like its author's cloud, and it changed before publication
rather than after — [ADR-0003](docs/adr/0003-contract-metrics-are-named-not-queried.md).
That is what `/v0` is for.

So: if the contract does not fit your substrate, that is a finding about the
contract, not about your substrate. Open an issue and say what does not fit.
Changing `/v0` to accommodate a real second implementation is the plan, not a
concession.

Start at [`docs/providers.md`](docs/providers.md), and prove your provider with
the conformance suite:

```bash
go run ./cmd/conformance -url https://your-provider.example -token "$TOKEN"
```

A provider that honestly declares a capability unimplemented passes. The suite
exits non-zero only on a genuine contract violation.

## Before you write code

**Read [`CONTEXT.md`](CONTEXT.md).** It is a glossary, and it is binding on
prose, identifiers, and commit messages alike. Words like *provider*, *capability*,
*substrate*, *instance*, and *app* have exact meanings here, and several
plausible synonyms are explicitly rejected — a patch calling a provider a
"plugin" or an app a "service" will get review comments about vocabulary before
it gets any about code.

If you think a term is wrong, that is worth an issue on its own. The glossary is
meant to be argued with; it is just meant to be argued with explicitly.

## Running it

Go 1.25 and access to a Kubernetes cluster — k3s, kind, minikube, anything. No
cloud account of any kind is required, and that is deliberate: if you cannot
contribute without one, the architecture has failed.

[`docs/providers.md`](docs/providers.md#running-it-locally) has the two-terminal
setup: the provider reading your cluster, and core reading the provider.

## Before you open a pull request

```bash
go vet ./...
go test ./...
```

CI runs both, plus the conformance suite against the in-tree providers. A
contract change that breaks an in-tree provider fails the build, which is the
point.

## Commit messages

Write the subject as a plain imperative sentence saying what the change does, in
the glossary's vocabulary:

```
Read infrastructure through providers, not the Oracle SDK
Correct the console's claim that MySQL is blocked
Stop documenting a build runner that no longer exists
```

No `feat:` / `fix:` prefixes. Older commits have them; they are history, not a
pattern to follow.

Use the body to say **why**, especially when the change is surprising, reverses
an earlier decision, or leaves something deliberately undone. A future reader
asking "why is it like this?" is the audience.

## Decisions

Some changes deserve an [ADR](docs/adr/). Write one when all three are true:

1. It would be expensive to reverse.
2. A future reader will wonder why it was done this way.
3. There were real alternatives and you picked one for specific reasons.

If any is missing, skip it. Four ADRs and a glossary are worth reading; forty
are not.

## What is unlikely to be merged

- **High availability, multi-region, or multi-tenant isolation.** These are
  non-goals under [ADR-0002](docs/adr/0002-constrained-first-scope.md), not
  gaps. TinyCloud targets clusters small enough to fit in a free tier.
- **Moving providers back in-process.** [ADR-0001](docs/adr/0001-providers-as-http-services.md)
  covers why they are HTTP services.
- **Large unsolicited refactors.** Open an issue first — not because they are
  unwelcome, but because a single maintainer cannot review one fairly against a
  branch they have not thought about.

## Legal

Apache-2.0, as in [`LICENSE`](LICENSE). There is no CLA; contributions are taken
under the licence the project ships.
