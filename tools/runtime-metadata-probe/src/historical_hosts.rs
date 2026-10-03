//! Deterministic storage hosts carry a job-owned operation and cumulative byte
//! budget. Hash/trie helpers are pure; crypto, offchain, keystore, randomness,
//! indexing, runtime spawning and other omitted hosts remain failing stubs.

use super::observer;
use sp_core::{
    storage::{well_known_keys::is_child_storage_key, ChildInfo, StateVersion},
    traits::Externalities,
};
use sp_externalities::ExternalitiesExt;
use sp_runtime_interface::{pass_by::*, runtime_interface};

pub(super) const HOST_PROFILE: &str = "substrate-proof-bounded-storage-v1";
const MAXIMUM_VALUE: usize = 8 * 1024 * 1024;
const MAXIMUM_CALLS: usize = 65536;
const MAXIMUM_IO: usize = 64 * 1024 * 1024;

/// Cumulative work is not refunded by a rollback or repeated access.
#[derive(Default)]
pub(super) struct Budget {
    pub calls: usize,
    pub io_bytes: usize,
    pub depth: usize,
}
sp_externalities::decl_extension! { pub(super) struct HistoricalBudget(Budget); }
impl Default for HistoricalBudget {
    fn default() -> Self {
        Self(Budget::default())
    }
}

fn charge(mut ext: &mut dyn Externalities, bytes: usize) {
    let budget = &mut ext
        .extension::<HistoricalBudget>()
        .expect("historical budget absent")
        .0;
    budget.calls += 1;
    budget.io_bytes = budget
        .io_bytes
        .checked_add(bytes)
        .expect("historical byte count overflow");
    assert!(
        budget.calls <= MAXIMUM_CALLS && budget.io_bytes <= MAXIMUM_IO,
        "historical storage work bound"
    );
}

fn key(key: &[u8]) {
    assert!(
        key.len() <= 512 && !is_child_storage_key(key),
        "historical storage key bound"
    );
}
fn child(key: &[u8]) -> ChildInfo {
    assert!(
        !key.is_empty() && key.len() <= 512,
        "historical child identity bound"
    );
    ChildInfo::new_default(key)
}
fn value(ext: &mut dyn Externalities, bytes: Option<Vec<u8>>) -> Option<Vec<u8>> {
    if let Some(value) = &bytes {
        assert!(
            value.len() <= MAXIMUM_VALUE,
            "historical storage value bound"
        );
        charge(ext, value.len());
    }
    bytes
}
fn copy_value(value: Option<Vec<u8>>, output: &mut [u8], offset: u32) -> Option<u32> {
    assert!(
        output.len() <= MAXIMUM_VALUE,
        "historical read output bound"
    );
    value.map(|value| {
        let remaining = &value[(offset as usize).min(value.len())..];
        let count = output.len().min(remaining.len());
        output[..count].copy_from_slice(&remaining[..count]);
        remaining.len() as u32
    })
}

#[runtime_interface]
pub trait Storage {
    fn get(
        &mut self,
        item: PassFatPointerAndRead<&[u8]>,
    ) -> AllocateAndReturnByCodec<Option<Vec<u8>>> {
        key(item);
        charge(*self, item.len());
        observer::observe(*self, "get", item, None);
        let result = self.storage(item);
        value(*self, result)
    }
    fn read(
        &mut self,
        item: PassFatPointerAndRead<&[u8]>,
        output: PassFatPointerAndReadWrite<&mut [u8]>,
        offset: u32,
    ) -> AllocateAndReturnByCodec<Option<u32>> {
        key(item);
        charge(*self, item.len());
        observer::observe(*self, "read", item, None);
        let result = self.storage(item);
        copy_value(value(*self, result), output, offset)
    }
    fn set(&mut self, item: PassFatPointerAndRead<&[u8]>, bytes: PassFatPointerAndRead<&[u8]>) {
        key(item);
        assert!(
            bytes.len() <= MAXIMUM_VALUE,
            "historical storage value bound"
        );
        charge(*self, item.len() + bytes.len());
        observer::observe(*self, "set", item, Some(bytes));
        self.set_storage(item.to_vec(), bytes.to_vec());
    }
    fn clear(&mut self, item: PassFatPointerAndRead<&[u8]>) {
        key(item);
        charge(*self, item.len());
        observer::observe(*self, "clear", item, None);
        self.clear_storage(item);
    }
    fn exists(&mut self, item: PassFatPointerAndRead<&[u8]>) -> bool {
        key(item);
        charge(*self, item.len());
        observer::observe(*self, "exists", item, None);
        self.exists_storage(item)
    }
    fn next_key(
        &mut self,
        item: PassFatPointerAndRead<&[u8]>,
    ) -> AllocateAndReturnByCodec<Option<Vec<u8>>> {
        key(item);
        charge(*self, item.len());
        observer::observe(*self, "next_key", item, None);
        let result = self.next_storage_key(item);
        value(*self, result)
    }
    fn append(
        &mut self,
        item: PassFatPointerAndRead<&[u8]>,
        bytes: PassFatPointerAndRead<Vec<u8>>,
    ) {
        key(item);
        assert!(
            bytes.len() <= MAXIMUM_VALUE,
            "historical storage value bound"
        );
        let previous = self.storage(item);
        let previous = value(*self, previous);
        assert!(
            previous.as_ref().map_or(0, Vec::len) + bytes.len() + 5 <= MAXIMUM_VALUE,
            "historical append value bound"
        );
        charge(*self, item.len() + bytes.len());
        observer::observe(*self, "append", item, Some(&bytes));
        self.storage_append(item.to_vec(), bytes);
    }
    fn root(&mut self) -> AllocateAndReturnFatPointer<Vec<u8>> {
        charge(*self, 0);
        self.storage_root(StateVersion::V0)
    }
    #[version(2)]
    fn root(&mut self, version: PassAs<StateVersion, u8>) -> AllocateAndReturnFatPointer<Vec<u8>> {
        charge(*self, 0);
        self.storage_root(version)
    }
    fn start_transaction(&mut self) {
        charge(*self, 0);
        let budget = &mut self
            .extension::<HistoricalBudget>()
            .expect("historical budget absent")
            .0;
        budget.depth += 1;
        assert!(budget.depth <= 32, "historical transaction depth bound");
        self.storage_start_transaction();
        observer::transaction(*self, "start");
    }
    fn rollback_transaction(&mut self) {
        charge(*self, 0);
        let budget = &mut self
            .extension::<HistoricalBudget>()
            .expect("historical budget absent")
            .0;
        assert!(budget.depth > 0, "historical transaction imbalance");
        budget.depth -= 1;
        self.storage_rollback_transaction()
            .expect("historical rollback failed");
        observer::transaction(*self, "rollback");
    }
    fn commit_transaction(&mut self) {
        charge(*self, 0);
        let budget = &mut self
            .extension::<HistoricalBudget>()
            .expect("historical budget absent")
            .0;
        assert!(budget.depth > 0, "historical transaction imbalance");
        budget.depth -= 1;
        self.storage_commit_transaction()
            .expect("historical commit failed");
        observer::transaction(*self, "commit");
    }
}

#[runtime_interface]
pub trait DefaultChildStorage {
    fn get(
        &mut self,
        owner: PassFatPointerAndRead<&[u8]>,
        item: PassFatPointerAndRead<&[u8]>,
    ) -> AllocateAndReturnByCodec<Option<Vec<u8>>> {
        let child = child(owner);
        key(item);
        charge(*self, owner.len() + item.len());
        let result = self.child_storage(&child, item);
        value(*self, result)
    }
    fn read(
        &mut self,
        owner: PassFatPointerAndRead<&[u8]>,
        item: PassFatPointerAndRead<&[u8]>,
        output: PassFatPointerAndReadWrite<&mut [u8]>,
        offset: u32,
    ) -> AllocateAndReturnByCodec<Option<u32>> {
        let child = child(owner);
        key(item);
        charge(*self, owner.len() + item.len());
        let result = self.child_storage(&child, item);
        copy_value(value(*self, result), output, offset)
    }
    fn set(
        &mut self,
        owner: PassFatPointerAndRead<&[u8]>,
        item: PassFatPointerAndRead<&[u8]>,
        bytes: PassFatPointerAndRead<&[u8]>,
    ) {
        let child = child(owner);
        key(item);
        assert!(
            bytes.len() <= MAXIMUM_VALUE,
            "historical storage value bound"
        );
        charge(*self, owner.len() + item.len() + bytes.len());
        self.set_child_storage(&child, item.to_vec(), bytes.to_vec());
    }
    fn clear(&mut self, owner: PassFatPointerAndRead<&[u8]>, item: PassFatPointerAndRead<&[u8]>) {
        let child = child(owner);
        key(item);
        charge(*self, owner.len() + item.len());
        self.clear_child_storage(&child, item);
    }
    fn exists(
        &mut self,
        owner: PassFatPointerAndRead<&[u8]>,
        item: PassFatPointerAndRead<&[u8]>,
    ) -> bool {
        let child = child(owner);
        key(item);
        charge(*self, owner.len() + item.len());
        self.exists_child_storage(&child, item)
    }
    fn next_key(
        &mut self,
        owner: PassFatPointerAndRead<&[u8]>,
        item: PassFatPointerAndRead<&[u8]>,
    ) -> AllocateAndReturnByCodec<Option<Vec<u8>>> {
        let child = child(owner);
        key(item);
        charge(*self, owner.len() + item.len());
        let result = self.next_child_storage_key(&child, item);
        value(*self, result)
    }
    fn root(
        &mut self,
        owner: PassFatPointerAndRead<&[u8]>,
    ) -> AllocateAndReturnFatPointer<Vec<u8>> {
        let child = child(owner);
        charge(*self, owner.len());
        self.child_storage_root(&child, StateVersion::V0)
    }
    #[version(2)]
    fn root(
        &mut self,
        owner: PassFatPointerAndRead<&[u8]>,
        version: PassAs<StateVersion, u8>,
    ) -> AllocateAndReturnFatPointer<Vec<u8>> {
        let child = child(owner);
        charge(*self, owner.len());
        self.child_storage_root(&child, version)
    }
}

pub(super) type HistoricalHostFunctions = (
    sp_io::allocator::HostFunctions,
    sp_io::logging::HostFunctions,
    sp_io::hashing::HostFunctions,
    sp_io::trie::HostFunctions,
    storage::HostFunctions,
    default_child_storage::HostFunctions,
);
