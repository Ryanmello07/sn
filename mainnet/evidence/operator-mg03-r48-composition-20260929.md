# MG03 and R48 server source composition

Server commit `34fec2baaa6cb972f0e8f9e2a5d9897407418517` is a full merge of
the former mainnet server root `0633780c` and the complete qualified MG03
lineage `26008e58`. Its tree `20791e914b91fa283bf88c83f850aef3a89d58f2`
is byte-identical to that qualified lineage. Both histories remain reachable,
including the three earlier controller recovery fixes that a narrow
cherry-pick would omit. The merged commit is provisionally on
`codex/mainnet-server-hardening-20260927` and pushed; it is not a deployed
server release.

Astra's [source-composition report](/mnt/data/sn-testnet/qualification/mg03-r48-server-composition-20260929/RESULT.md)
(SHA-256 `328e4a537dbfa09481ad971e56bee8ac0a920a49a1b75ed16be9b249aa6f15d8`)
preserves the six original merge conflicts, their resolutions, exact tree and
module identities, historical approvals and a 134-root affected test manifest.
Every conflict resolves to the already qualified source bytes. The separate
test-only successor `05fee56f3051b8b96ff00f16bbf7f994ad57404d` adds two
deterministic composition roots for settlement/archive custody and
registration/policy-history recovery, taking the target manifest to 136 roots.
Both test branches are isolated from the merged server root.

The merger ran formatting, diff and model compile-only checks. Sol's fresh
normal/race behavioral qualification and composed release build are pending.
The exact prior MG03 receipts remain valid for their original source graph;
tree equality and compilation do not prove the new two-root composition or a
production rollout. No live database, chain, signer or deployment was touched.
