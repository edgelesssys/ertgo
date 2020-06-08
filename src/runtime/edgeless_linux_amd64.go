// Copyright 2020 Edgeless Systems GmbH. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

const (
	_CLOCK_REALTIME = 0
	_ETIMEDOUT      = 110
)

type semt [4]uint64

// sema functions copied from os_aix.go

//go:nosplit
func semacreate(mp *m) {
	if mp.waitsema != 0 {
		return
	}

	var sem *semt

	// Call libc's malloc rather than malloc. This will
	// allocate space on the C heap. We can't call mallocgc
	// here because it could cause a deadlock.
	sem = (*semt)(malloc(unsafe.Sizeof(*sem)))
	if sem_init(sem, 0, 0) != 0 {
		throw("sem_init")
	}
	mp.waitsema = uintptr(unsafe.Pointer(sem))
}

//go:nosplit
func semasleep(ns int64) int32 {
	_m_ := getg().m
	if ns >= 0 {
		var ts timespec

		if clock_gettime(_CLOCK_REALTIME, &ts) != 0 {
			throw("clock_gettime")
		}
		ts.tv_sec += ns / 1e9
		ts.tv_nsec += ns % 1e9
		if ts.tv_nsec >= 1e9 {
			ts.tv_sec++
			ts.tv_nsec -= 1e9
		}

		if r, err := sem_timedwait((*semt)(unsafe.Pointer(_m_.waitsema)), &ts); r != 0 {
			if err == _ETIMEDOUT || err == _EAGAIN || err == _EINTR {
				return -1
			}
			println("sem_timedwait err ", err, " ts.tv_sec ", ts.tv_sec, " ts.tv_nsec ", ts.tv_nsec, " ns ", ns, " id ", _m_.id)
			throw("sem_timedwait")
		}
		return 0
	}
	for {
		r1, err := sem_wait((*semt)(unsafe.Pointer(_m_.waitsema)))
		if r1 == 0 {
			break
		}
		if err == _EINTR {
			continue
		}
		throw("sem_wait")
	}
	return 0
}

//go:nosplit
func semawakeup(mp *m) {
	if sem_post((*semt)(unsafe.Pointer(mp.waitsema))) != 0 {
		throw("sem_post")
	}
}

//go:linkname asmsysvicall6x runtime.asmsysvicall6
var asmsysvicall6x uintptr // name to take addr of asmsysvicall6

func asmsysvicall6()       // declared for vet; do NOT call
func invoke_libc_syscall() // declared for vet; do NOT call

//go:nosplit
func sysvicall6(fn *uintptr, nargs, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, int32) {
	call := libcall{
		fn:   uintptr(unsafe.Pointer(fn)),
		n:    nargs,
		args: uintptr(unsafe.Pointer(&a1)),
	}
	asmcgocall(unsafe.Pointer(&asmsysvicall6x), unsafe.Pointer(&call))
	return call.r1, int32(call.err)
}

var (
	libc_clock_gettime,
	libc_malloc,
	libc_sem_init,
	libc_sem_post,
	libc_sem_timedwait,
	libc_sem_wait uintptr
)

//go:linkname libc_clock_gettime libc_clock_gettime
//go:linkname libc_malloc libc_malloc
//go:linkname libc_sem_init libc_sem_init
//go:linkname libc_sem_post libc_sem_post
//go:linkname libc_sem_timedwait libc_sem_timedwait
//go:linkname libc_sem_wait libc_sem_wait

//go:nosplit
func clock_gettime(clockid int32, tp *timespec) int32 {
	r, _ := sysvicall6(&libc_clock_gettime, 2, uintptr(clockid), uintptr(unsafe.Pointer(tp)), 0, 0, 0, 0)
	return int32(r)
}

//go:nosplit
func malloc(size uintptr) unsafe.Pointer {
	r, _ := sysvicall6(&libc_malloc, 1, size, 0, 0, 0, 0, 0)
	return unsafe.Pointer(r)
}

//go:nosplit
func sem_init(sem *semt, pshared int32, value uint32) int32 {
	r, _ := sysvicall6(&libc_sem_init, 3, uintptr(unsafe.Pointer(sem)), uintptr(pshared), uintptr(value), 0, 0, 0)
	return int32(r)
}

//go:nosplit
func sem_post(sem *semt) int32 {
	r, _ := sysvicall6(&libc_sem_post, 1, uintptr(unsafe.Pointer(sem)), 0, 0, 0, 0, 0)
	return int32(r)
}

//go:nosplit
func sem_timedwait(sem *semt, timeout *timespec) (int32, int32) {
	r, e := sysvicall6(&libc_sem_timedwait, 2, uintptr(unsafe.Pointer(sem)), uintptr(unsafe.Pointer(timeout)), 0, 0, 0, 0)
	return int32(r), e
}

//go:nosplit
func sem_wait(sem *semt) (int32, int32) {
	r, e := sysvicall6(&libc_sem_wait, 1, uintptr(unsafe.Pointer(sem)), 0, 0, 0, 0, 0)
	return int32(r), e
}
