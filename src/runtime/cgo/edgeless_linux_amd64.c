// Copyright 2020 Edgeless Systems GmbH. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <errno.h>
#include <time.h>
#include <unistd.h>

long libc_syscall(long number, long a1, long a2, long a3, long a4, long a5, long a6) {
	return syscall(number, a1, a2, a3, a4, a5, a6);
}

int* libc_errno(void) {
	return &errno;
}

int libc_clock_gettime(clockid_t clockid, struct timespec* tp) {
	return clock_gettime(clockid, tp);
}
