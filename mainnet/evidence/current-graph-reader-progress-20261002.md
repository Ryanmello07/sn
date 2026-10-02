# Current dependency graph and guarded-reader progress

This is scoped source evidence, not release or deployment authorization.

Server `22e3c1ba438b7888dccfc04244bcafaae0bcad3d` / tree
`b6cb4a0dd35d22a909e17c599e6995fd873c3aa2` independently passes
12 selected local-blob tests normally and with race detection, plus vet.
The actual declared graph uses published Connect `e0d75562`, SCTP `6443417d`
and archived SN `69f4bbdd`. Root rehashed all 26 independent manifest bindings.
The [independent receipt](current-server-module-independent-20261002.json)
has SHA-256 `121dcefd3b72d92466c4db3f4028a73adf826eed987723679645e04b6ce37ff2`.
The earlier `cebf154f` graph failure remains retained; tests never ran on it.

Server upstream subsequently advanced to `87712b3b` in 28 other files.
Review composition `32196d57ab5253691f38fbc119991097b013df05` / tree
`5853f647b0ff8eefad8715c0dcb2cdf1d6eee7d7` preserves those changes and the
exact eight blob/module files from `22e3c1ba`. Its current-graph qualification
is pending. Neither an upstream merge nor unchanged selected files inherit
qualification of the complete composition. Server main has not adopted it.

Guarded spool reader `68a7be85502ed7a0fd139afcd2f212178cdec019` passes
15 author normal/15 race roots and validator vet. Root rehashed all 40
[author receipt](guarded-reader-author-20261002.json) bindings; its SHA-256 is
`1feef2e3d7b5299f737538369de81760339b24e51a2b3a21e20d5b01ce15b814`.
The earlier `261efc9f` correction preserved bare EOF but allowed a full buffer
count with a failed custody guard. ReadFull and JSON consumers could accept
that count despite the error. Four causal controls reproduce this on `261efc9f`
in each mode; four partial-read and production-descriptor positive controls
pass. The corrective reader returns zero admitted bytes when its post-read
custody guard fails, preserving the actual advanced descriptor position.
Independent qualification is queued. This source is not merged into SN main.
The production descriptor reader already checks the read error before decoding;
the consumer probe does not establish existing production corruption.

The current SN retained-member composition `5f1fe123` is independently testing
ten observer, preparation, retained-member and execution seams on its exact
qualified Server `10a8f4d8` / Connect `0a5cda0e` graph. It is separate from the
new published-module intake above. Full PH/MG acceptance remains open.
