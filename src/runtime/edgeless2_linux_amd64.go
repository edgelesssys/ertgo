// Copyright 2026 Edgeless Systems GmbH. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import _ "unsafe"

//go:linkname invoke_libc_syscall
func invoke_libc_syscall()
