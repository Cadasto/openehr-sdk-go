---
title: Workflow
description: >-
  Where the openEHR Go SDK sits between a modelling tool and a CDR — compile
  and check in process, then write and query over REST.
---

# Workflow

Compile a template, validate a Composition, and encode it without touching the
network; reach for the REST client only when you have somewhere to send the
result. This page walks one path from a modelling tool to a clinical data
repository (CDR) and marks which steps the SDK covers.

<div class="workflow" markdown="1">

<p class="legend" markdown="1">
<span class="scope scope--in">In the SDK</span> shipped packages you import
<span class="scope scope--out">Outside</span> a modelling tool or a live CDR
</p>

<div class="product-card" markdown="1">

<span class="scope scope--out">Outside</span>

### 1. Model
Author archetypes and an operational template (OPT) in a clinical modelling
tool, then export the OPT as ADL 1.4 `.opt`. The SDK does not replace that
tool. It starts from the file you already have.

</div>

<div class="product-card" markdown="1">

<span class="scope scope--in">In the SDK</span>

### 2. Work in process
Parse the OPT and compile it, all without a network. Export Web Template JSON
for FLAT paths. Build a Composition, or synthesise one. Validate it against
the compiled template. Encode canonical JSON, canonical XML, FLAT, or
STRUCTURED. Build or parse AQL and lint it.

Packages: `template`, `templatecompile`, `validation`, and the codecs under
`serialize/`. The full map is on [Packages](packages.md).

[Run a building-block example](examples.md){ .md-button }

</div>

<div class="features-grid" markdown="1">

<div class="feature-card" markdown="1">

:material-file-code:

### Parse and compile
`template` reads the `.opt`. `templatecompile.Compile` returns the handle the
builder, the synthesiser, the validator, and AQL lint all accept.

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
`composition` is an OPT-driven builder. When you need synthetic data instead,
`instance` fills a skeleton from the compiled template.

</div>

<div class="feature-card" markdown="1">

:material-swap-horizontal:

### Encode what the CDR accepts
Canonical JSON and XML, plus FLAT and STRUCTURED in both directions, driven by
the exported Web Template.

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
Attach one `auth.TokenSource` — SMART-on-openEHR, client credentials, JWT
bearer, or HTTP Basic. Then create an EHR, commit a Composition, or batch
several versions through a contribution. Point the service catalog at your
[ITS-REST](https://specifications.openehr.org/releases/ITS-REST/Release-1.1.0/)
CDR.

In tests you have two ways to avoid a real CDR. Injecting `sandbox.Backend` as
the client's `http.RoundTripper` answers requests in memory, with no server at
all; the examples take the other route and start a loopback `httptest` server,
which needs no credentials.

Packages: `client/ehr` and the providers under `auth/`.

[Create an EHR offline](examples.md#create-an-ehr){ .md-button }

</div>

<div class="product-card" markdown="1">

<span class="scope scope--in">In the SDK</span>

### 5. Read and query
Build AQL from structs or parse it from text, then run it through
`client/query`. The result set is a typed model, and cells that hold RM
objects decode as RM types.

</div>

</div>

## Which CDR {#which-cdr}

The REST client follows the ITS-REST Release-1.1.0 OpenAPI files kept in this
repository, not any vendor's own SDK, so it can point at any CDR implementing
that contract. Wider interoperability work is still under way, and we publish
no compatibility matrix.

`make test` never dials a CDR. The Live probes skip unless you set the
variables below.

| Deployment | Notes |
|---|---|
| **openEHR REST** | The contract is ITS-REST Release-1.1.0. The OpenAPI files are in [`resources/its-rest/`](https://github.com/cadasto/openehr-sdk-go/blob/main/resources/its-rest/README.md). |
| **Cadasto** | Cadasto B.V. writes this SDK. Point the client at a Cadasto CDR the same way as any other ITS-REST base. The extra APIs under `cadasto/` are listed on [Packages](packages.md#cadasto-extras). |
| **EHRbase** | Web Template export and the FLAT codec follow the EHRbase reference implementation ([ADR 0014](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/adr/0014-webtemplate-reference-implementation-lock.md)). Set `OPENEHR_LIVE_EHRBASE` to run the Live probes against an instance; we have run the opt-in Live snapshots against EHRbase 2.35.1 locally, and they are not part of CI. |
| **FerroEHR** | Set `OPENEHR_LIVE_FERROEHR` to name the deployment and `OPENEHR_LIVE_FERROEHR_BASIC` to carry its `user:pass` credential, then run the Live probes. They are not part of CI. |
| **Better Platform** | **Work in progress.** There is no Live probe yet. Web Template export emits EHRbase ids (`blood_pressure`) rather than Better camelCase ids (`bloodPressure`) — [ADR 0014](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/adr/0014-webtemplate-reference-implementation-lock.md). |

## Terms on this page

| Name | What it is here |
|---|---|
| OPT | ADL 1.4 operational template (XML) from a modelling tool |
| Web Template | JSON path map the SDK exports from a compiled OPT |
| Canonical JSON / XML | openEHR ITS encodings with a `_type` discriminator |
| FLAT / STRUCTURED | Simplified formats, bidirectional, Web-Template-driven |
| AQL | Archetype Query Language — build, parse, lint, then execute |
| RM | Reference Model types generated from the BMM this SDK builds against |
| TokenSource | One interface for SMART, client credentials, JWT bearer, and Basic |

The import map is on [Packages](packages.md), and the generated API is on
[Go reference](reference.md).
