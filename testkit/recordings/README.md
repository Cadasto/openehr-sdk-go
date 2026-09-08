# testkit/recordings

REQ-082 Cassette-mode HTTP Archive 1.2 recordings ([ADR 0020](../../docs/adr/0020-cassette-recording-har.md)).

These are whole exchanges — method, URL, headers, status, bodies — not the
request/response *bodies* under [`testkit/cassettes/`](../cassettes/). A
recording without `log._req082` provenance or with `redaction.ran` other
than `true` is discarded, not replayed ([ValidateHAR](../probe/har.go)).

| File | Capture |
|---|---|
| [ehr-create.har](ehr-create.har) | Live EHRbase 2.35.1 `POST /ehr` (STRAND-11 evidence, byte-identical to [`docs/plans/strand-11-evidence/ehr-create.har`](../../docs/plans/strand-11-evidence/ehr-create.har)) |

Replay through [`probe.NewReplayer`](../probe/replay.go). Capture through
[`probe.NewRecorder`](../probe/record.go). Do not hand-edit a recording to
invent a response — recapture, or the replay no longer witnesses a
deployment.
