# Owner-trim optional proxy storage repair

The first isolated action candidate was
`403360920027c1b0d9e67f615d63a75a294fc9e7`, tree
`950d392dd3cf4471e441b6d9bd00b480c24f96ba`, on top of qualified readiness
SN `e35771ec47aa8d486ce2af03a83eb3edace9cbde`. Sol medium's focused normal
and race runs each failed
`TestOwnerTrimAuthorityCurrentPruningBoundaryNeverGrantsEnforcement` with
`state_getStorage: missing result`. The original source, failed receipts and
first handoff remain unchanged. The first selector includes 20 newly added
roots and two pre-existing command roots, for 22 selected roots total.

The root cause is the new production `Proxy.Proxies` call site. Its synthetic
fixture already emits an explicit JSON `result: null` for an absent storage
key. The ordinary RPC call requires a concrete result and therefore rejects
that valid absence. The repair uses the existing explicit optional-storage
reader after authenticating the proxy metadata, map key and default tuple.
It retains the finite read budget and 1 MiB reply limit. The shared RPC
decoder and fixture are unchanged: a missing result field, wrong JSON type,
RPC error or transport failure cannot become a storage absence witness.

Three additional top-level regression roots cover the actual current-window
path: null versus stored empty tuple with distinct retained evidence; omitted
and wrongly typed results at exactly the proxy read after a valid census and
nonce; and present unresolved proxy rows, including nonzero deposits and
wrong tuple lengths. An adjacent source audit found all other optional storage
readers already use explicit nullable storage profiles. No blanket nullable
RPC policy was introduced.

Finished failure evidence under
`/mnt/data/sn-testnet/qualification/mg08-owner-trim-phase-20260929/`:

| Receipt | SHA-256 |
| --- | --- |
| `focus-normal.json` | `49a2a6373fc72682ffb967307711233b3c328a24d6fc6f24cbc501b92f5dc04e` |
| `focus-race.json` | `244f783c1fab0cee81a384520adbd14e9525b66c202c14181d046fab561577d0` |
| `expanded-normal.json` | `567927e7567e3dcce8c99cc808894408e5e23de226595415db6d2bb3e88ba54c` |
| `astra/HANDOFF.md` | `3bd4418fdbc4afb03dc1fcb632f2be83475dcc844d70a37d0ae897db182adf2b` |

The successor's exact source, compile-only checks and unchanged physical module
pins are recorded in
`/mnt/data/sn-testnet/qualification/mg08-owner-trim-phase-r2-20260929/astra/HANDOFF.md`.
Its static census is 23 newly added roots relative to e35771ec, 25 selected
focused roots and 229 roots in the expanded union. Sol owns behavioral and
race qualification, including reintroducing the old non-nullable proxy call
as a causal control. Those results must be sealed against the exact successor
before integration; compile and vet are not behavioral qualification.

This repair grants no signing or submission authority. Original v3 custody,
the independent action approval, original nonce/era/allowance and all protected
generation requirements remain intact. The absence of a future window enforcer
still blocks production execution; no pending best-effort risk-policy choice
is inferred. Financial finality remains separate from generation reconciliation
and explicit old-miner residuals. MG-08 and live launch acceptance remain open.
