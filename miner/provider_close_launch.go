// Launch generators and embedded provider owners share the actual device
// settings constructor; optional signing never changes ordinary key admission.
package miner

import "github.com/urnetwork/sdk"

// Fresh settings preserve sdk defaults and copy only the already owned domain.
// A zero domain retains legacy unsigned close reports.
func ProviderDeviceSettings(domainHash [32]byte) *sdk.DeviceLocalSettings {
	settings := sdk.DefaultDeviceLocalSettings()
	settings.ClientSettings.ContractManagerSettings.CloseReportDomainHash = domainHash
	return settings
}

// Reads the optional launch artifact once. Both the real CLI and a launcher
// receive the same bounded descriptor/parser result; errors return zero domain.
func ReadProviderCloseReportDomain(path string) ([32]byte, error) {
	return readProviderCloseReportDomain(path)
}
