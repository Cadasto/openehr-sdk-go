---
title: openEHR Go SDK
description: >-
  Cadasto's Go SDK for openEHR: typed Reference Model, canonical codecs,
  operational templates you can validate against, AQL, and SMART-on-openEHR
  auth. The building-block packages import without the HTTP client.
hide:
  - navigation
  - toc
template: home.html
---

<div class="home-hero" markdown="1">

![](assets/logo.svg){ .home-hero__mark }

# openEHR Go SDK

<p class="home-tagline">
Typed compositions, checkable templates, and AQL — imported like any other Go package.
</p>

<div class="home-cta" markdown="1">

[Install](install.md){ .md-button .md-button--primary }
[View on GitHub](https://github.com/cadasto/openehr-sdk-go){ .md-button }

</div>

<p class="home-reassure" markdown="1">
No backend needed to start. Point the client at yours when you have one.
</p>

</div>

<h2 class="section-title">openEHR and this SDK</h2>

<div class="two-products" markdown="1">

<div class="product-card" markdown="1">

### What openEHR is

openEHR is an open specification for an electronic health record. Clinical
content is modelled on two levels: a stable
[Reference Model](https://specifications.openehr.org/releases/RM/development/ehr.html)
that every record shares, and archetypes that constrain it for one purpose.
Creating an EHR, uploading a template, running a query — those platform
operations come from the
[Service Model](https://specifications.openehr.org/releases/SM/development/openehr_platform.html).
This SDK is a Go client for the
[ITS-REST Release-1.1.0](https://specifications.openehr.org/releases/ITS-REST/Release-1.1.0/)
binding of that platform, so it can talk to any clinical data repository (CDR)
that implements that binding.

[Which CDR →](workflow.md#which-cdr)

</div>

<div class="product-card" markdown="1">

### Who it is for

Go developers meeting openEHR, and openEHR developers meeting Go. It was
written with a few kinds of consumer in mind: benchmark and load tools running
high-concurrency CRUD, synthetic data seeders that drive bulk Compositions from
a template, MCP servers that forward the caller's token, federative clients
that fan out over several CDRs, and SMART-on-openEHR apps with a Go backend.

You do not have to adopt the whole module. If the job only needs one
building-block package, import that one.

</div>

</div>

<h2 class="section-title">What you import</h2>

<div class="features-grid" markdown="1">

<div class="feature-card" markdown="1">

:material-package-variant:

### Import only what you use
Reference Model types, codecs, validation, templates, and AQL are ordinary Go
packages, and none of them reaches for `transport/` or `auth/`. A CI job that
validates a Composition takes no network dependency.

</div>

<div class="feature-card" markdown="1">

:material-language-go:

### Idiomatic Go
The public surface reads like any other Go module: short package names,
constructors that return errors, and plain types you pass around. There is no
framework to adopt first.

</div>

<div class="feature-card" markdown="1">

:material-hexagon-outline:

### Typed Reference Model
Compositions and data values are generated Go structs, not maps of strings.
After a decode you read `c.Name` and `len(c.Content)`, and a misspelt field is
a compile error rather than a surprise at runtime.

</div>

<div class="feature-card" markdown="1">

:material-file-check:

### Templates you can check
Parse an ADL 1.4 operational template (OPT), compile it, and validate a
Composition against it before anything reaches a CDR.

</div>

<div class="feature-card" markdown="1">

:material-code-tags:

### AQL as data
Archetype Query Language (AQL) queries are ordinary values here. Build one from structs
or parse one from text, then lint it against a compiled template before you
send it.

</div>

<div class="feature-card" markdown="1">

:material-key-chain:

### One TokenSource
SMART-on-openEHR, client credentials, JWT bearer, and HTTP Basic all implement
`auth.TokenSource`. Attach one to the client, or override it for a single
request.

</div>

</div>

<h2 class="section-title">Two ways in</h2>

<div class="two-products" markdown="1">

<div class="product-card" markdown="1">

:material-cube-outline:

### Building blocks
Decode canonical JSON, parse an OPT, validate a Composition, build or lint AQL.
None of it goes near the network.

[See the packages](packages.md){ .md-button }

</div>

<div class="product-card" markdown="1">

:material-api:

### REST client
The REST client packages cover EHR, composition, query, and definition. Point
them at the in-process mock the examples use, or at a live openEHR REST
deployment.

[Run an example](examples.md){ .md-button }

</div>

</div>

<div class="quick-start" markdown="1">

## Quick start

```bash
go get github.com/cadasto/openehr-sdk-go@v0.27.0
```

```go
var c rm.Composition
if err := canjson.Unmarshal(body, &c); err != nil {
    log.Fatal(err)
}
fmt.Println(c.ArchetypeNodeID, c.Category.Value, len(c.Content))
```

Requires Go 1.27 or newer. The SDK is pre-1.0, so pin an exact tag: a minor
release can change the public API.

[Install →](install.md) · [Examples →](examples.md) · [Go reference →](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)

</div>
