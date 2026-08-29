// System Pack registration for the replay-worker binary.
//
// The worker selects the pack from PACK_IMPL via the same pack-agnostic registry
// as the Core API. Registration is the only place the worker learns about a
// concrete pack; PACK_IMPL=checkout_duplicate_effect resolves the real
// checkout_duplicate_effect pack here. Core/replay logic is never edited for an
// added pack.
package main

import (
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/packregistry"
	"github.com/causalens/causalens/internal/systempack/checkout"
)

func init() {
	packregistry.RegisterDefault(checkout.PackID, func() contracts.SystemPack {
		return checkout.New()
	})
	packregistry.RegisterDefault(packregistry.DevImplementation, func() contracts.SystemPack {
		return packregistry.NewDevPack()
	})
}
