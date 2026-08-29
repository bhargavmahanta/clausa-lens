// Package main wiring for the Core API's System Pack selection.
//
// This file is the single, owned wiring point for the Core API. The dev pack is
// registered here so the capsule/run/diff routes are exercisable end-to-end
// without Member 1's checkout pack; it is selectable only via PACK_IMPL=dev.
// When Member 1's real pack lands, register it here with its implementation
// token (to Replace/coexist with the dev entry) rather than editing a core
// package:
//
//	packregistry.RegisterDefault("checkout_duplicate_effect", checkout.New)
//
// Core, capsule and differential logic are never edited for that integration.
package main

import (
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/packregistry"
)

func init() {
	packregistry.RegisterDefault(packregistry.DevImplementation, func() contracts.SystemPack {
		return packregistry.NewDevPack()
	})
}
