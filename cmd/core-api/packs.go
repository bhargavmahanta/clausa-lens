// Package main wiring for the Core API's System Pack selection.
//
// This file is the single, owned wiring point for the Core API. Both packs are
// registered here so the capsule/run/diff routes are exercisable end-to-end:
// the dev pack is selectable via PACK_IMPL=dev, and Member 1's real
// checkout_duplicate_effect pack is selected via PACK_IMPL=checkout_duplicate_effect
// (or any deployment token the integration branch chooses). The dev entry
// never replaces the real one; they coexist as separate tokens.
//
//	packregistry.RegisterDefault(packregistry.DevImplementation, ...)
//	packregistry.RegisterDefault(checkout.PackID, checkout.New)
//
// Core, capsule and differential logic are never edited for that integration.
package main

import (
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/packregistry"
	"github.com/causalens/causalens/internal/systempack/checkout"
)

func init() {
	packregistry.RegisterDefault(packregistry.DevImplementation, func() contracts.SystemPack {
		return packregistry.NewDevPack()
	})
	packregistry.RegisterDefault(checkout.PackID, func() contracts.SystemPack {
		return checkout.New()
	})
}
