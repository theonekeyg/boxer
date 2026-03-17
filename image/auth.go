package image

import "github.com/google/go-containerregistry/pkg/authn"

// defaultKeychain returns the default credential keychain, which reads
// ~/.docker/config.json and well-known credential helpers.
func defaultKeychain() authn.Keychain {
	return authn.DefaultKeychain
}
