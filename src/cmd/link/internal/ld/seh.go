// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ld

import (
	"cmd/internal/sys"
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
)

var sehp struct {
	pdata loader.Sym
	xdata loader.Sym
}

func writeSEH(ctxt *Link) {
	switch ctxt.Arch.Family {
	case sys.AMD64:
		writeSEHAMD64(ctxt)
	}
}

func writeSEHAMD64(ctxt *Link) {
	ldr := ctxt.loader
	mkSecSym := func(name string, kind sym.SymKind) *loader.SymbolBuilder {
		s := ldr.CreateSymForUpdate(name, 0)
		s.SetType(kind)
		s.SetAlign(4)
		return s
	}
	pdata := mkSecSym(".pdata", sym.SSEHSECT)
	xdata := mkSecSym(".xdata", sym.SSEHSECT)
	// The .xdata entries have very low cardinality
	// as it only contains frame pointer operations,
	// which are very similar across functions.
	// These are referenced by .pdata entries using
	// an RVA, so it is possible, and binary-size wise,
	// to deduplicate .xdata entries.
	uwcache := make(map[string]int64) // aux symbol name --> .xdata offset
	for _, s := range ctxt.Textp {
		if fi := ldr.FuncInfo(s); !fi.Valid() || fi.TopFrame() {
			continue
		}
		uw := ldr.SEHUnwindSym(s)
		if uw == 0 {
			continue
		}
		name := ctxt.SymName(uw)
		off, cached := uwcache[name]
		if !cached {
			off = xdata.Size()
			uwcache[name] = off
			xdata.AddBytes(ldr.Data(uw))
		}

		// Reference:
		// https://learn.microsoft.com/en-us/cpp/build/exception-handling-x64#struct-runtime_function
		pdata.AddPEImageRelativeAddrPlus(ctxt.Arch, s, 0)
		pdata.AddPEImageRelativeAddrPlus(ctxt.Arch, s, ldr.SymSize(s))
		pdata.AddPEImageRelativeAddrPlus(ctxt.Arch, xdata.Sym(), off)
	}
	sehp.pdata = pdata.Sym()
	sehp.xdata = xdata.Sym()
}
