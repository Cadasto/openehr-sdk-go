# testkit/recordings

REQ-082 Cassette-mode HTTP Archive 1.2 recordings ([ADR 0020](../../docs/adr/0020-cassette-recording-har.md)).

These are whole exchanges (method, URL, headers, status, bodies), unlike the
request/response *bodies* under [`testkit/cassettes/`](../cassettes/). A
recording without `log._req082` provenance or with `redaction.ran` other
than `true` is discarded, not replayed ([ValidateHAR](../probe/har.go)).

| File | Capture | Recapture |
|---|---|---|
| [ehr-create.har](ehr-create.har) | Live EHRbase 2.35.1 `POST /ehr` (STRAND-11 evidence, byte-identical to [`docs/plans/strand-11-evidence/ehr-create.har`](../../docs/plans/strand-11-evidence/ehr-create.har)) | hand-captured before the harness existed; no `-scenario` covers it |
| [ehr-lifecycle.har](ehr-lifecycle.har) | Live EHRbase 2.35.1 `POST /ehr`, then `GET` and `HEAD` the created id (the create-then-read path) | `-scenario ehr-lifecycle` |

Replay through [`probe.NewReplayer`](../probe/replay.go).

Capture with `make probe-record`, which runs the
[`cmd/probe-record`](../../cmd/probe-record) harness. The harness drives one of its
registered scenarios against a live CDR through
[`probe.NewRecorder`](../probe/record.go) and gates the result before writing
anything. While the captured document is still in memory, the harness validates it
([`probe.HAR.Validate`](../probe/har.go)) and replays it against a different base
URL. Only approved bytes are written, to a temp file that is then renamed into
place. A capture that leaks a credential therefore never reaches disk, and one
that cannot be replayed never replaces the recording already there.

`make probe-record` stamps `sdk_commit` from `git rev-parse HEAD`, with a
`-dirty` suffix when the tree has uncommitted changes. A recording captured
from a modified tree says so, instead of naming a commit whose code did not
produce it. A corpus recording should carry a clean commit.

Do not hand-edit a recording to invent a response. Recapture instead; otherwise
the replay no longer witnesses a deployment.
