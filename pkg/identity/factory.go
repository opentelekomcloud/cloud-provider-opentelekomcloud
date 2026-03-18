package identity

import (
	"fmt"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

// NewIdentityProvider creates the appropriate IdentityProvider based on
// configured credentials. Priority: AK/SK > Password.
func NewIdentityProvider(opts config.AuthOpts) (IdentityProvider, error) {
	switch opts.AuthMethod() {
	case config.AuthMethodAKSK:
		return NewAKSKProvider(opts)
	case config.AuthMethodPassword:
		return NewPasswordProvider(opts)
	default:
		return nil, fmt.Errorf("no valid auth credentials found: provide username/password or access-key/secret-key")
	}
}
