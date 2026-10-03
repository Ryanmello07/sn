// Operational growth is an explicit reviewed revision of the original policy,
// not a new contract/census identity. Previously acknowledged limits cannot shrink.
package main

import "errors"

type monitorEvmResources struct {
	ReadBudgetSeconds uint64 `json:"read_budget_seconds"`
	HistoryEntries    uint64 `json:"history_entries"`
	BatchBlocks       uint64 `json:"batch_blocks"`
	StallSeconds      uint64 `json:"stall_seconds"`
}

type monitorEvmResourceRevision struct {
	Original     monitorEvmResources `json:"original"`
	ReviewSha256 string              `json:"review_sha256"`
}

func (self monitorEconomicEvmPolicy) resources() monitorEvmResources {
	return monitorEvmResources{ReadBudgetSeconds: self.ReadBudgetSeconds, HistoryEntries: self.HistoryEntries, BatchBlocks: self.BatchBlocks, StallSeconds: self.StallSeconds}
}

func (self monitorEvmResources) seconds() uint64 {
	if self.ReadBudgetSeconds == 0 {
		return 300
	}
	return self.ReadBudgetSeconds
}

func (self monitorEvmResources) validate() error {
	if self.seconds() < 60 || self.seconds() > 900 || self.HistoryEntries == 0 || self.HistoryEntries > maximumMonitorEconomicEvents || self.BatchBlocks == 0 || self.BatchBlocks > maximumMonitorEvmBlocks || self.StallSeconds < 60 || self.StallSeconds > 3600 {
		return errors.New("EVM economic resource revision exceeds reviewed bounds")
	}
	return nil
}

func (self monitorEvmResources) includes(prior monitorEvmResources) bool {
	return self.seconds() >= prior.seconds() && self.HistoryEntries >= prior.HistoryEntries && self.BatchBlocks >= prior.BatchBlocks && self.StallSeconds >= prior.StallSeconds
}

func (self monitorEconomicEvmPolicy) validateResourceRevision() error {
	if err := self.resources().validate(); err != nil {
		return err
	}
	if self.ResourceRevision == nil {
		return nil
	}
	if !planSha256(self.ResourceRevision.ReviewSha256) || self.ResourceRevision.Original.validate() != nil || !self.resources().includes(self.ResourceRevision.Original) {
		return errors.New("EVM economic resource renewal lacks review or shrinks its original basis")
	}
	return nil
}

func (self *monitorEconomicEvmPolicy) setResources(value monitorEvmResources) {
	self.ReadBudgetSeconds, self.HistoryEntries, self.BatchBlocks, self.StallSeconds = value.ReadBudgetSeconds, value.HistoryEntries, value.BatchBlocks, value.StallSeconds
}

func (self monitorEconomicEvmPolicy) identityHash() string {
	if self.ResourceRevision != nil {
		self.setResources(self.ResourceRevision.Original)
		self.ResourceRevision = nil
	}
	return rootObjectHash(self)
}
