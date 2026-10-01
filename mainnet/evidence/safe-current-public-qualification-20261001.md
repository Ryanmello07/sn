# Public Safe current-only submission qualification

This October 1 increment adds a domain-separated v2 risk-policy acceptance and
an exact accepted-revision opt-in to the public bootstrap successor command.
V1 remains public read-only. No production policy acceptance, real signer or live
transaction is part of this work. The original complete-history route remains
unimplemented and distinct.

Qualification is in progress on the isolated branch based on SN `1806b3b3`.
The two new acceptance/custody roots and public native-proof command root pass
initial normal checks; expanded normal/race checks and causal controls must finish
before the final receipt is sealed. Initial public command package: PASS 89.571s.
Author vet passes. These preliminary checks do not claim deployment readiness.

The deterministic public command fixture uses synthetic independent signatures,
published Safe bytecode, the retained eight-action local graph and authenticated
native trie witnesses. It covers absent acceptance, absent/wrong opt-in, v1 public
refusal, hidden owner/module authority, reorgs, advancing proof/admission heads,
late pending Safe/relayer nonce and runtime changes, a consumed reservation after
proof-time mutation, exact lost-reply execution and read-only receipt recovery.
The separate acceptance tests cover independent signing domains and exact partial
publication recovery from v1 to v2, with unchanged earlier counted authority,
original custody, nonces and maximum liabilities across runtime revision.

The actual production independent v2 risk-policy acceptance, owned-route/current
runtime qualification, exclusive signer cutover, complete custody and live chain
readback remain gates. Current-only proof does not establish complete historical
initialization/delegatecalls, independent finality or an authenticated pending
overlay; its scoped pending reads cannot exclude changes between calls.
