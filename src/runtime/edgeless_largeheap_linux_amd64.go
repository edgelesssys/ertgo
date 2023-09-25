// Copyright 2021 Edgeless Systems GmbH. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ego_largeheap

package runtime

// These values are moved from malloc.go to here to be able
// to change them depending on the ego_largeheap build tag.
const (
	// Original value is 48. Assume that user space is <= 7fff'ffff'ffff
	// and use 47 instead to halve the space of the heap bitmap.
	heapAddrBits = 47

	logHeapArenaBytes = 26 // this value is unmodified
	arenaBaseOffset   = 0  // must be 0 so that full user space can be addressed with 47 bits
)

func edgmallocinit() {
}
