//go:build !linux && !darwin

package clientauth

import "errors"

// Renewal publishes through descriptor-relative custody, which only Unix
// platforms provide here; elsewhere the token is left to its explicit sign-in.
func RenewNetworkToken(string, string, string) (bool, error) {
	return false, errors.New("network token renewal requires Unix custody support")
}
