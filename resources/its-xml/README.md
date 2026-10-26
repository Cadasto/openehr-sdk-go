# `resources/its-xml/` — pinned openEHR ITS-XML schemas

Vendored, version-pinned XML Schemas (XSD) from the openEHR ITS-XML specification: the RM schemas that define canonical XML, and the ADL 1.4 operational template (OPT) schema. The canonical XML codec (`openehr/serialize/canxml`, whose marshalling code `internal/bmmgen` generates) and the OPT parser (`openehr/template`) are written against them.

**A reference only.** No code is generated from these files and no test reads them. They are here so a reviewer or an agent checks an element name, an attribute-versus-element choice, a type or an occurrence in the published schema rather than guessing, the way [`../its-rest/`](../its-rest/README.md) serves the REST API. `make its-xml-check` keeps them byte-identical to upstream.

## Provenance

[`MANIFEST.txt`](MANIFEST.txt) records the upstream commit of [`openEHR/specifications-ITS-XML`](https://github.com/openEHR/specifications-ITS-XML) and each file's `sha256`. [`scripts/sync-its-xml.sh`](../../scripts/sync-its-xml.sh) writes it. Each file sits at its upstream path below `components/`, so the relative `xs:include` locations between the schemas resolve inside this directory, and the sync refuses a file set that is not closed under them.

Two facts about the upstream set matter when reading it against the SDK:

- **RM release.** The SDK generates from RM 1.2.0 ([`../bmm/`](../bmm/README.md)), but the newest RM release ITS-XML publishes schemas for is `Release-1.1.0`. Its `latest` RM directory is the same schema (it differs only in the line endings of `Demographic.xsd`), and the AM 1.4 schemas include `Release-1.1.0`, so that release is the one pinned.
- **Namespace.** Every pinned schema declares `targetNamespace="http://schemas.openehr.org/v2"`. The SDK reads and writes `http://schemas.openehr.org/v1`, the namespace of OPTs and canonical XML documents in circulation. Validating an SDK document against these schemas therefore needs the namespace rewritten first.

## Files

| Path | Role |
|---|---|
| `RM/Release-1.1.0/*.xsd`, `RM/Release-1.1.0/documents/*.xsd` | Canonical XML of the RM: data types, data structures, common, EHR, demographic, extract, and the document roots (`Composition.xsd`, `Version.xsd`, …) |
| `BASE/latest/BaseTypes.xsd` | BASE types the RM schemas include |
| `AM/Release-1.4/Template.xsd` | The ADL 1.4 operational template (`<template>`), including `OpenehrProfile.xsd`, `Archetype.xsd` and `Resource.xsd` |

The pinned commit also carries upstream's SPECPR-492 change to the AM 1.4 schemas (ordinal and scale constraints that accept the empty symbol text some tools write) and its SPECPR-432 change (`C_PRIMITIVE_OBJECT` item).

## Updating the pin

```bash
make its-xml-drift                     # changed files and newer releases upstream (network)
ITS_XML_REF=master make its-xml-sync   # move the pin to the tip of master
make its-xml-check                     # offline sha256 check; part of make ci
```

A bare `make its-xml-sync` re-fetches the commit already pinned and never moves it. The upstream release tags predate the current AM 1.4 schemas, so a re-pin is normally `ITS_XML_REF=master`. The file set is the `SRC_PATHS` list in the script; to take a newer RM release, edit the list there, and the sync drops the files the list no longer names. Never edit a schema or the manifest by hand.

## Licence

Apache-2.0, the licence of the upstream repository ([`LICENSE`](https://github.com/openEHR/specifications-ITS-XML/blob/master/LICENSE)); a full copy is [`../../licenses/Apache-2.0.txt`](../../licenses/Apache-2.0.txt). Repository-wide inventory: [`LICENSING.md`](../../LICENSING.md).
