//! Original Wasm captures the transaction from its actual encoded body and
//! moves its computed pool stake. Receipt and contract execution are separate
//! Go consumers; these synthetic programs confer no deployed-runtime authority.

use super::*;

#[test]
fn historical_native_vault_capture_exports_original_transaction_causes() {
    let mut jobs = Vec::new();
    for (name, after, committed, discarded) in [
        ("capture", 0, 2, 0),
        ("capture-causes", 0, 5, 0),
        ("capture-unclassified", 0, 4, 0),
        ("capture-rollback", 20, 1, 2),
    ] {
        let (job, _) = fixture_with_principal_effects(false, Some(Some(14)), Some(name));
        let captured = super::super::capture_tests::collect(&job)
            .expect("actual original vault capture program");
        let exported: HistoricalJob = serde_json::from_str(&captured.job_json).unwrap();
        let replay = run(&exported).expect("actual original vault replay program");
        for report in [&captured.replay, &replay] {
            assert!(report.post_state_reproduced);
            assert_eq!(report.extrinsics, 1);
            assert_eq!(
                report.closing_principals.as_ref().unwrap()[0].opening_stake_alpha,
                Some(after.to_string())
            );
            let trace = report.hook_observations.as_ref().unwrap();
            assert_eq!(trace.principal_mutations.as_ref().unwrap().len(), committed);
            assert_eq!(trace.discarded_on_rollback, discarded);
            let captures: Vec<_> = trace
                .observations
                .iter()
                .filter(|value| value.purpose == "native-principal-vault-capture")
                .collect();
            assert_eq!(
                captures.len(),
                usize::from(name != "capture-rollback"),
                "original committed capture census"
            );
            for value in captures {
                let native = value.native.as_ref().unwrap();
                assert_eq!(native.execution_phase_hex.as_deref(), Some("0x0000000000"));
                let transaction = native
                    .memory
                    .iter()
                    .find(|value| value.name == "transaction-hash")
                    .unwrap();
                assert_eq!(
                    transaction.bytes_hex,
                    encoded(&[0x99; 32]),
                    "transaction came from original body"
                );
            }
        }
        jobs.push((name, exported));
    }
    if let Some(directory) = std::env::var_os("URNETWORK_NATIVE_VAULT_CAPTURE_FIXTURE_OUT") {
        let directory = Path::new(&directory);
        assert!(directory.is_absolute() && directory.is_dir());
        for (name, job) in jobs {
            let mut file = OpenOptions::new()
                .create_new(true)
                .write(true)
                .mode(0o600)
                .open(directory.join(format!("principal-{name}.json")))
                .unwrap();
            file.write_all(&serde_json::to_vec(&job).unwrap()).unwrap();
            file.sync_all().unwrap();
        }
        std::fs::File::open(directory).unwrap().sync_all().unwrap();
    }
}
