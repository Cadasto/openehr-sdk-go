---
title: openEHR Go SDK
description: >-
  A first-party Go SDK for openEHR — typed Reference Model, canonical codecs,
  OPT-driven compositions, AQL, and SMART-on-openEHR auth. Building-block
  packages import without the HTTP client.
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

<h2 class="section-title">What openEHR is</h2>

<div class="product-card" markdown="1">

openEHR is an open specification for an electronic health record.
Clinical content uses two-level modelling: a stable
[Reference Model](https://specifications.openehr.org/releases/RM/development/ehr.html)
and archetypes that constrain it. Platform operations — EHR, definition,
query — are defined by the
[Service Model](https://specifications.openehr.org/releases/SM/development/openehr_platform.html).
This SDK is a Go client for the
[ITS-REST](https://specifications.openehr.org/releases/ITS-REST/Release-1.1.0/)
binding of that platform (the pin this repository calls openEHR REST
`1.1.0`).

[Which CDR →](workflow.md#which-cdr)

</div>

<h2 class="section-title">What you import</h2>

<div class="features-grid" markdown="1">

<div class="feature-card" markdown="1">

:material-package-variant:

### Import only what you use
RM types, codecs, validation, templates, and AQL are ordinary packages. They do not pull `transport/` or `auth/`. A CI job that validates a Composition never takes a network dependency.

</div>

<div class="feature-card" markdown="1">

:material-language-go:

### Idiomatic Go
The public surface reads like the rest of a Go module: short package names, constructors that return errors, and types you pass around without a framework.

</div>

<div class="feature-card" markdown="1">

:material-hexagon-outline:

### Typed Reference Model
Compositions and data values are generated Go structs, not maps of strings. You read `c.Name` and `len(c.Content)` after a decode.

</div>

<div class="feature-card" markdown="1">

:material-file-check:

### Templates you can check
Parse an ADL 1.4 OPT, compile it, and validate a Composition against it before anything reaches a CDR.

</div>

<div class="feature-card" markdown="1">

:material-code-tags:

### AQL as data
Build a query from structs or parse one from text. Lint it before you send it.

</div>

<div class="feature-card" markdown="1">

:material-key-chain:

### One TokenSource
SMART-on-openEHR, client credentials, JWT bearer, and HTTP Basic implement one interface. Attach it once, or swap it per request.

</div>

</div>

<h2 class="section-title">Two ways in</h2>

<div class="two-products" markdown="1">

<div class="product-card" markdown="1">

:material-cube-outline:

### Building blocks
Decode JSON, parse an OPT, validate a Composition, build or lint AQL — no network.

[See the packages](packages.md){ .md-button }

</div>

<div class="product-card" markdown="1">

:material-api:

### REST client
Typed leaves for EHR, composition, query, and definition. Point at a mock or a live openEHR REST deployment.

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

Pin the tag: the SDK is pre-1.0. The module floor is Go **1.27.x**.

[Install →](install.md) · [Examples →](examples.md) · [Go reference →](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)

</div>
