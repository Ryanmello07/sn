package clientauth

import "io"

// readCredentialFile reads a credential the custody may replace or set aside
// at any moment; see openCredentialFile.
func readCredentialFile(path string) (_ []byte, returnErr error) {
	file, err := openCredentialFile(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); returnErr == nil {
			returnErr = err
		}
	}()
	return io.ReadAll(file)
}
