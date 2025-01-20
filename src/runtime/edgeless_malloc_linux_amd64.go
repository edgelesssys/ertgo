// Copyright 2021 Edgeless Systems GmbH. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !ego_largeheap

package runtime

import "unsafe"

// These values are moved from malloc.go to here to be able
// to change them depending on the ego_largeheap build tag.
const (
	// Original value is 48. Decrease to 35 to reduce the
	// size of the heap bitmap and allow smaller enclaves.
	heapAddrBits = 35

	// Original value is 26. Reduce to 22 to allow smaller enclaves.
	// (Also see original comment in malloc.go.)
	logHeapArenaBytes = 22
)

// Original value is const. We need to determine this at runtime because
// we don't have enough heapAddrBits to cover the whole address space.
var arenaBaseOffset uintptr

func edgmallocinit() {
	// check if the preferred arena base address is usable
	const _MAP_FIXED_NOREPLACE = 0x10_0000
	const preferredBase uintptr = 0xC0_0000_0000
	p, errno := mmap(unsafe.Pointer(preferredBase), 1, _PROT_NONE, _MAP_PRIVATE|_MAP_ANON|_MAP_FIXED_NOREPLACE, -1, 0)
	if errno == 0 {
		munmap(p, 1)
		if uintptr(p) == preferredBase {
			edgSetArenaBase(preferredBase)
			return
		}
	}

	// get some address in the mmappable space
	p, errno = mmap(nil, 1, _PROT_NONE, _MAP_PRIVATE|_MAP_ANON, -1, 0)
	if errno != 0 {
		println("ego runtime: mmap failed with", errno)
		throw("ego runtime: mmap failed")
	}
	munmap(p, 1)

	// heuristic that's sufficient for enclaves up to 16GB:
	// set arena base so that p is in the middle of the arena space
	const baseAlign = 0x40_0000
	edgSetArenaBase(uintptr(p)&^(baseAlign-1) - maxAlloc/2)
}

func edgSetArenaBase(base uintptr) {
	arenaBaseOffset = base
	// see mranges.go
	minOffAddr = offAddr{arenaBaseOffset}
	maxOffAddr = offAddr{(((1 << heapAddrBits) - 1) + arenaBaseOffset) & uintptrMask}
}
