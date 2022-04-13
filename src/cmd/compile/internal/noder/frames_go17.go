// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.7
// +build go1.7

package noder

import "runtime"

func walkFrames(pcs []uintptr, visit frameVisitor) {
	if len(pcs) == 0 {
		return
	}

	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		visit(frame.File, frame.Line, frame.Function, frame.PC-frame.Entry)
		if !more {
			return
		}
	}
}
