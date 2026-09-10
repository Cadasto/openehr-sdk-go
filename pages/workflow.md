---
title: Workflow
description: >-
  Where the openEHR Go SDK sits between a modelling tool and a CDR —
  compile and check in process, then write and query over REST.
---

# Workflow

Where this SDK sits between a modelling tool and a clinical data
repository (CDR). You can compile, validate, and encode without a
network. The REST leaves are optional.

<div class="workflow" markdown="1">

<p class="legend" markdown="1">
<span class="scope scope--in">In the SDK</span> shipped packages you import
<span class="scope scope--out">Outside</span> a modelling tool or a live CDR
</p>

<div class="product-card" markdown="1">

<span class="scope scope--out">Outside</span>

### 1. Model
Author archetypes and an operational template in a clinical modelling
tool, then export an ADL 1.4 `.opt`. The SDK does not replace that
tool. It starts from the file you already have.

</div>

<div class="product-card" markdown="1">

<span class="scope scope--in">In the SDK</span>

### 2. Work in process
Parse the OPT, compile it, and stay off the network. Export WebTemplate
JSON for FLAT paths. Build or synthesise a Composition. Validate it
against the compiled template. Encode canonical JSON, canonical XML,
FLAT, or STRUCTURED. Build or parse AQL and lint it.

Packages: `template`, `templatecompile`, `composition`, `instance`,
`validation`, `serialize/canjson`, `serialize/canxml`,
`serialize/simplified`, `aql`, `aql/parse`, `aql/lint`.

[Run a building-block example](examples.md){ .md-button }

</div>

<div class="features-grid" markdown="1">

<div class="feature-card" markdown="1">

:material-file-code:

### Parse and compile
`template` reads the `.opt`. `templatecompile.Compile` is the handle
the builder, the synthesiser, the validator, and AQL lint accept.

</div>

<div class="feature-card" markdown="1">

:material-check-decagram:

### Check before you send
`validation.ValidateComposition` reports constraint issues against the
compiled OPT. `ValidateRM` walks an RM root with no template at all.

</div>

<div class="feature-card" markdown="1">

:material-cube-scan:

### Build or synthesise
`composition` is an OPT-driven builder. `instance` fills a skeleton
from the compiled template when you need synthetic data.

</div>

<div class="feature-card" markdown="1">

:material-swap-horizontal:

### Encode what the CDR accepts
Canonical JSON and XML, plus bidirectional FLAT and STRUCTURED driven
by the exported Web Template.

</div>

</div>

<div class="product-card" markdown="1">

<span class="scope scope--in">In the SDK</span>

### 3. Publish the template
`definition.UploadTemplate` sends the ADL 1.4 OPT to an openEHR REST
definition API. ADL 2 upload is **deferred**.

</div>

<div class="product-card" markdown="1">

<span class="scope scope--in">In the SDK</span>

### 4. Write
Attach one `auth.TokenSource` — SMART-on-openEHR, client credentials,
JWT bearer, or HTTP Basic. Create an EHR, commit a Composition, or
batch versions through a contribution. Point the catalog at your
[ITS-REST](https://specifications.openehr.org/releases/ITS-REST/Release-1.1.0/)
backend. Which CDRs this repository records is under
[Which CDR](#which-cdr). For tests, inject `sandbox.Backend` as the
client's `http.RoundTripper`, or use the in-process mock the examples
stand up. Nothing in that path needs a listener.

Leaves: `client/ehr`, `client/ehr/composition`,
`client/ehr/contribution`. Auth: `auth`, `auth/smart`,
`auth/clientcreds`, `auth/jwtbearer`, `auth/basic`.

[Create an EHR offline](examples.md#create-an-ehr){ .md-button }

</div>

<div class="product-card" markdown="1">

<span class="scope scope--in">In the SDK</span>

### 5. Read and query
Build AQL from structs or parse it from text, then run it through
`client/query`. The result set is a typed model; cells that are RM
objects decode as RM types.

</div>

</div>

## Which CDR {#which-cdr}

The REST client follows the vendored
[ITS-REST](https://specifications.openehr.org/releases/ITS-REST/Release-1.1.0/)
OpenAPI pin, not a vendor SDK. Point it at any CDR that implements
that contract. Broader interop is **work in progress** — this
repository does not publish a compatibility matrix.

`make test` does not dial a CDR. Live probes skip unless you set the
variables below.

| Deployment | What this repository records |
|---|---|
| **openEHR REST** | The pin is ITS-REST Release-1.1.0, in [`resources/its-rest/`](https://github.com/cadasto/openehr-sdk-go/blob/main/resources/its-rest/README.md). |
| **Cadasto** | Cadasto B.V. writes this SDK. Point the client at a Cadasto CDR the same way as any other ITS-REST base. Extra APIs under `cadasto/` (Datamap, MPI, Extra API, Care) are **planned** or **partial** — they are not the openEHR REST surface. |
| **EHRbase** | Web Template and FLAT follow the EHRbase reference ([ADR 0014](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/adr/0014-webtemplate-reference-implementation-lock.md)). Set `OPENEHR_LIVE_EHRBASE` to run Live probes against an instance. Checking a running deployment that way is still open. |
| **FerroEHR** | Set `OPENEHR_LIVE_FERROEHR` to run Live probes. The helper strips `/ferroehr/rest/openehr/v1` so the client can use the usual REST paths. Those probes are not in CI. |
| **Better Platform** | **Work in progress.** There is no Live probe yet. Web Template export still uses EHRbase ids (`blood_pressure`), not Better camelCase ids (`bloodPressure`) — [ADR 0014](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/adr/0014-webtemplate-reference-implementation-lock.md). |

## Formats this page names

| Name | What it is here |
|---|---|
| OPT | ADL 1.4 operational template (XML) from a modelling tool |
| Web Template | JSON path map the SDK exports from a compiled OPT |
| Canonical JSON / XML | openEHR ITS encodings with a `_type` discriminator |
| FLAT / STRUCTURED | Simplified formats, bidirectional, Web-Template-driven |
| AQL | Archetype Query Language — build, parse, lint, then execute |
| RM | Reference Model types generated from the pinned BMM |
| TokenSource | One interface for SMART, client credentials, JWT bearer, and Basic |

## What this page does not claim

Clinical modelling stays in the tool that produced the `.opt`. Cadasto
platform extras under `cadasto/` are **planned** or **partial** and are
not the openEHR REST surface. The SDK is pre-1.0: pin a module tag.

The import map is on [Packages](packages.md). The generated API is
[Go reference](reference.md).
