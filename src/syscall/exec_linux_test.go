// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package syscall_test

import (
	"bytes"
	"flag"
	"fmt"
	"internal/testenv"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// whoamiNEWUSER returns a command that runs "whoami" with CLONE_NEWUSER,
// mapping uid and gid 0 to the actual uid and gid of the test.
func whoamiNEWUSER(t *testing.T, uid, gid int, setgroups bool) *exec.Cmd {
	t.Helper()
	testenv.MustHaveExecPath(t, "whoami")
	cmd := testenv.Command(t, "whoami")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: gid, Size: 1},
		},
		GidMappingsEnableSetgroups: setgroups,
	}
	return cmd
}

func TestCloneNEWUSERAndRemap(t *testing.T) {
	for _, setgroups := range []bool{false, true} {
		setgroups := setgroups
		t.Run(fmt.Sprintf("setgroups=%v", setgroups), func(t *testing.T) {
			uid := os.Getuid()
			gid := os.Getgid()

			cmd := whoamiNEWUSER(t, uid, gid, setgroups)
			out, err := cmd.CombinedOutput()
			t.Logf("%v: %v", cmd, err)

			if uid != 0 && setgroups {
				t.Logf("as non-root, expected permission error due to unprivileged gid_map")
				if !os.IsPermission(err) {
					if err == nil {
						t.Skipf("unexpected success: probably old kernel without security fix?")
					}
					if testenv.SyscallIsNotSupported(err) {
						t.Skipf("skipping: CLONE_NEWUSER appears to be unsupported")
					}
					t.Fatalf("got non-permission error") // Already logged above.
				}
				return
			}

			if err != nil {
				if testenv.SyscallIsNotSupported(err) {
					// May be inside a container that disallows CLONE_NEWUSER.
					t.Skipf("skipping: CLONE_NEWUSER appears to be unsupported")
				}
				t.Fatalf("unexpected command failure; output:\n%s", out)
			}

			sout := strings.TrimSpace(string(out))
			want := "root"
			if sout != want {
				t.Fatalf("whoami = %q; want %q", out, want)
			}
		})
	}
}

func TestEmptyCredGroupsDisableSetgroups(t *testing.T) {
	cmd := whoamiNEWUSER(t, os.Getuid(), os.Getgid(), false)
	cmd.SysProcAttr.Credential = &syscall.Credential{}
	if err := cmd.Run(); err != nil {
		if testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: %v: %v", cmd, err)
		}
		t.Fatal(err)
	}
}

func TestUnshare(t *testing.T) {
	path := "/proc/net/dev"
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("kernel doesn't support proc filesystem")
		}
		if os.IsPermission(err) {
			t.Skip("unable to test proc filesystem due to permissions")
		}
		t.Fatal(err)
	}

	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	origLines := strings.Split(strings.TrimSpace(string(orig)), "\n")

	cmd := testenv.Command(t, "cat", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNET,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if testenv.SyscallIsNotSupported(err) {
			// CLONE_NEWNET does not appear to be supported.
			t.Skipf("skipping due to permission error: %v", err)
		}
		t.Fatalf("Cmd failed with err %v, output: %s", err, out)
	}

	// Check there is only the local network interface
	sout := strings.TrimSpace(string(out))
	if !strings.Contains(sout, "lo:") {
		t.Fatalf("Expected lo network interface to exist, got %s", sout)
	}

	lines := strings.Split(sout, "\n")
	if len(lines) >= len(origLines) {
		t.Fatalf("Got %d lines of output, want <%d", len(lines), len(origLines))
	}
}

func TestGroupCleanup(t *testing.T) {
	testenv.MustHaveExecPath(t, "id")
	cmd := testenv.Command(t, "id")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: 0,
			Gid: 0,
		},
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: %v: %v", cmd, err)
		}
		t.Fatalf("Cmd failed with err %v, output: %s", err, out)
	}
	strOut := strings.TrimSpace(string(out))
	t.Logf("id: %s", strOut)

	expected := "uid=0(root) gid=0(root)"
	// Just check prefix because some distros reportedly output a
	// context parameter; see https://golang.org/issue/16224.
	// Alpine does not output groups; see https://golang.org/issue/19938.
	if !strings.HasPrefix(strOut, expected) {
		t.Errorf("expected prefix: %q", expected)
	}
}

func TestGroupCleanupUserNamespace(t *testing.T) {
	testenv.MustHaveExecPath(t, "id")
	cmd := testenv.Command(t, "id")
	uid, gid := os.Getuid(), os.Getgid()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER,
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: gid, Size: 1},
		},
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: %v: %v", cmd, err)
		}
		t.Fatalf("Cmd failed with err %v, output: %s", err, out)
	}
	strOut := strings.TrimSpace(string(out))
	t.Logf("id: %s", strOut)

	// As in TestGroupCleanup, just check prefix.
	// The actual groups and contexts seem to vary from one distro to the next.
	expected := "uid=0(root) gid=0(root) groups=0(root)"
	if !strings.HasPrefix(strOut, expected) {
		t.Errorf("expected prefix: %q", expected)
	}
}

// Test for https://go.dev/issue/19661: unshare fails because systemd
// has forced / to be shared
func TestUnshareMountNameSpace(t *testing.T) {
	testenv.MustHaveExec(t)

	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		dir := flag.Args()[0]
		err := syscall.Mount("none", dir, "proc", 0, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "unshare: mount %v failed: %#v", dir, err)
			os.Exit(2)
		}
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	d := t.TempDir()
	t.Cleanup(func() {
		// If the subprocess fails to unshare the parent directory, force-unmount it
		// so that the test can clean it up.
		if _, err := os.Stat(d); err == nil {
			syscall.Unmount(d, syscall.MNT_FORCE)
		}
	})
	cmd := testenv.Command(t, exe, "-test.run=TestUnshareMountNameSpace", d)
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Unshareflags: syscall.CLONE_NEWNS}

	o, err := cmd.CombinedOutput()
	if err != nil {
		if testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: could not start process with CLONE_NEWNS: %v", err)
		}
		t.Fatalf("unshare failed: %v\n%s", err, o)
	}

	// How do we tell if the namespace was really unshared? It turns out
	// to be simple: just try to remove the directory. If it's still mounted
	// on the rm will fail with EBUSY.
	if err := os.Remove(d); err != nil {
		t.Errorf("rmdir failed on %v: %v", d, err)
	}
}

// Test for Issue 20103: unshare fails when chroot is used
func TestUnshareMountNameSpaceChroot(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		dir := flag.Args()[0]
		err := syscall.Mount("none", dir, "proc", 0, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "unshare: mount %v failed: %#v", dir, err)
			os.Exit(2)
		}
		os.Exit(0)
	}

	d := t.TempDir()

	// Since we are doing a chroot, we need the binary there,
	// and it must be statically linked.
	testenv.MustHaveGoBuild(t)
	x := filepath.Join(d, "syscall.test")
	t.Cleanup(func() {
		// If the subprocess fails to unshare the parent directory, force-unmount it
		// so that the test can clean it up.
		if _, err := os.Stat(d); err == nil {
			syscall.Unmount(d, syscall.MNT_FORCE)
		}
	})

	cmd := testenv.Command(t, testenv.GoToolPath(t), "test", "-c", "-o", x, "syscall")
	cmd.Env = append(cmd.Environ(), "CGO_ENABLED=0")
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Build of syscall in chroot failed, output %v, err %v", o, err)
	}

	cmd = testenv.Command(t, "/syscall.test", "-test.run=TestUnshareMountNameSpaceChroot", "/")
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: d, Unshareflags: syscall.CLONE_NEWNS}

	o, err := cmd.CombinedOutput()
	if err != nil {
		if testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: could not start process with CLONE_NEWNS and Chroot %q: %v", d, err)
		}
		t.Fatalf("unshare failed: %v\n%s", err, o)
	}

	// How do we tell if the namespace was really unshared? It turns out
	// to be simple: just try to remove the executable. If it's still mounted
	// on, the rm will fail.
	if err := os.Remove(x); err != nil {
		t.Errorf("rm failed on %v: %v", x, err)
	}
	if err := os.Remove(d); err != nil {
		t.Errorf("rmdir failed on %v: %v", d, err)
	}
}

// Test for Issue 29789: unshare fails when uid/gid mapping is specified
func TestUnshareUidGidMapping(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		defer os.Exit(0)
		if err := syscall.Chroot(os.TempDir()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	if os.Getuid() == 0 {
		t.Skip("test exercises unprivileged user namespace, fails with privileges")
	}

	testenv.MustHaveExec(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cmd := testenv.Command(t, exe, "-test.run=TestUnshareUidGidMapping")
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags:               syscall.CLONE_NEWNS | syscall.CLONE_NEWUSER,
		GidMappingsEnableSetgroups: false,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      syscall.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      syscall.Getgid(),
				Size:        1,
			},
		},
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: could not start process with CLONE_NEWNS and CLONE_NEWUSER: %v", err)
		}
		t.Fatalf("Cmd failed with err %v, output: %s", err, out)
	}
}

func prepareCgroupFD(t *testing.T) (int, string) {
	t.Helper()

	const O_PATH = 0x200000 // Same for all architectures, but for some reason not defined in syscall for 386||amd64.

	// Requires cgroup v2.
	const prefix = "/sys/fs/cgroup"
	selfCg, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			t.Skip(err)
		}
		t.Fatal(err)
	}

	// Expect a single line like this:
	// 0::/user.slice/user-1000.slice/user@1000.service/app.slice/vte-spawn-891992a2-efbb-4f28-aedb-b24f9e706770.scope
	// Otherwise it's either cgroup v1 or a hybrid hierarchy.
	if bytes.Count(selfCg, []byte("\n")) > 1 {
		t.Skip("cgroup v2 not available")
	}
	cg := bytes.TrimPrefix(selfCg, []byte("0::"))
	if len(cg) == len(selfCg) { // No prefix found.
		t.Skipf("cgroup v2 not available (/proc/self/cgroup contents: %q)", selfCg)
	}

	// Need clone3 with CLONE_INTO_CGROUP support.
	_, err = syscall.ForkExec("non-existent binary", nil, &syscall.ProcAttr{
		Sys: &syscall.SysProcAttr{
			UseCgroupFD: true,
			CgroupFD:    -1,
		},
	})
	if testenv.SyscallIsNotSupported(err) {
		t.Skipf("clone3 with CLONE_INTO_CGROUP not available: %v", err)
	}

	// Need an ability to create a sub-cgroup.
	subCgroup, err := os.MkdirTemp(prefix+string(bytes.TrimSpace(cg)), "subcg-")
	if err != nil {
		// ErrPermission or EROFS (#57262) when running in an unprivileged container.
		// ErrNotExist when cgroupfs is not mounted in chroot/schroot.
		if os.IsNotExist(err) || testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Rmdir(subCgroup) })

	cgroupFD, err := syscall.Open(subCgroup, O_PATH, 0)
	if err != nil {
		t.Fatal(&os.PathError{Op: "open", Path: subCgroup, Err: err})
	}
	t.Cleanup(func() { syscall.Close(cgroupFD) })

	return cgroupFD, "/" + path.Base(subCgroup)
}

func TestUseCgroupFD(t *testing.T) {
	testenv.MustHaveExec(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	fd, suffix := prepareCgroupFD(t)

	cmd := testenv.Command(t, exe, "-test.run=TestUseCgroupFDHelper")
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		UseCgroupFD: true,
		CgroupFD:    fd,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Cmd failed with err %v, output: %s", err, out)
	}
	// NB: this wouldn't work with cgroupns.
	if !bytes.HasSuffix(bytes.TrimSpace(out), []byte(suffix)) {
		t.Fatalf("got: %q, want: a line that ends with %q", out, suffix)
	}
}

func TestUseCgroupFDHelper(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	// Read and print own cgroup path.
	selfCg, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Print(string(selfCg))
}

func TestCloneTimeNamespace(t *testing.T) {
	testenv.MustHaveExec(t)

	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		timens, err := os.Readlink("/proc/self/ns/time")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Print(string(timens))
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cmd := testenv.Command(t, exe, "-test.run=TestCloneTimeNamespace")
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWTIME,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if testenv.SyscallIsNotSupported(err) {
			// CLONE_NEWTIME does not appear to be supported.
			t.Skipf("skipping, CLONE_NEWTIME not supported: %v", err)
		}
		t.Fatalf("Cmd failed with err %v, output: %s", err, out)
	}

	// Inode number of the time namespaces should be different.
	// Based on https://man7.org/linux/man-pages/man7/time_namespaces.7.html#EXAMPLES
	timens, err := os.Readlink("/proc/self/ns/time")
	if err != nil {
		t.Fatal(err)
	}

	parentTimeNS := string(timens)
	childTimeNS := string(out)
	if childTimeNS == parentTimeNS {
		t.Fatalf("expected child time namespace to be different from parent time namespace: %s", parentTimeNS)
	}
}

type capHeader struct {
	version uint32
	pid     int32
}

type capData struct {
	effective   uint32
	permitted   uint32
	inheritable uint32
}

const CAP_SYS_TIME = 25
const CAP_SYSLOG = 34

type caps struct {
	hdr  capHeader
	data [2]capData
}

func getCaps() (caps, error) {
	var c caps

	// Get capability version
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.hdr)), uintptr(unsafe.Pointer(nil)), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	// Get current capabilities
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.hdr)), uintptr(unsafe.Pointer(&c.data[0])), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	return c, nil
}

func TestAmbientCaps(t *testing.T) {
	testAmbientCaps(t, false)
}

func TestAmbientCapsUserns(t *testing.T) {
	testAmbientCaps(t, true)
}

func testAmbientCaps(t *testing.T, userns bool) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		caps, err := getCaps()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if caps.data[0].effective&(1<<uint(CAP_SYS_TIME)) == 0 {
			fmt.Fprintln(os.Stderr, "CAP_SYS_TIME unexpectedly not in the effective capability mask")
			os.Exit(2)
		}
		if caps.data[1].effective&(1<<uint(CAP_SYSLOG&31)) == 0 {
			fmt.Fprintln(os.Stderr, "CAP_SYSLOG unexpectedly not in the effective capability mask")
			os.Exit(2)
		}
		os.Exit(0)
	}

	// skip on android, due to lack of lookup support
	if runtime.GOOS == "android" {
		t.Skip("skipping test on android; see Issue 27327")
	}

	u, err := user.Lookup("nobody")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.ParseInt(u.Uid, 0, 32)
	if err != nil {
		t.Fatal(err)
	}
	gid, err := strconv.ParseInt(u.Gid, 0, 32)
	if err != nil {
		t.Fatal(err)
	}

	// Copy the test binary to a temporary location which is readable by nobody.
	f, err := os.CreateTemp("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.Close()
		os.Remove(f.Name())
	})

	testenv.MustHaveExec(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	e, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := io.Copy(f, e); err != nil {
		t.Fatal(err)
	}
	if err := f.Chmod(0755); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := testenv.Command(t, f.Name(), "-test.run="+t.Name())
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
		AmbientCaps: []uintptr{CAP_SYS_TIME, CAP_SYSLOG},
	}
	if userns {
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWUSER
		const nobody = 65534
		uid := os.Getuid()
		gid := os.Getgid()
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{
			ContainerID: int(nobody),
			HostID:      int(uid),
			Size:        int(1),
		}}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{
			ContainerID: int(nobody),
			HostID:      int(gid),
			Size:        int(1),
		}}

		// Set credentials to run as user and group nobody.
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: nobody,
			Gid: nobody,
		}
	}
	if err := cmd.Run(); err != nil {
		if testenv.SyscallIsNotSupported(err) {
			t.Skipf("skipping: %v: %v", cmd, err)
		}
		t.Fatal(err.Error())
	}
}
