// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
//   For website terms of use, trademark policy, privacy policy and other
//   project policies see https://lfprojects.org/policies
// Copyright (c) 2019-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package actions

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/apptainer/apptainer/e2e/internal/e2e"
	"github.com/apptainer/apptainer/e2e/internal/testhelper"
	"github.com/apptainer/apptainer/internal/pkg/test/tool/exec"
	"github.com/apptainer/apptainer/internal/pkg/test/tool/require"
	"github.com/apptainer/apptainer/internal/pkg/util/fs"
	"github.com/pkg/errors"
)

type actionTests struct {
	env e2e.TestEnv
}

// run tests min functionality for singularity symlink using actionRun
func (c actionTests) singularityLink(t *testing.T) {
	saveCmdPath := c.env.CmdPath
	i := strings.LastIndex(saveCmdPath, "apptainer")
	c.env.CmdPath = saveCmdPath[:i] + "singularity"

	c.actionRun(t)

	c.env.CmdPath = saveCmdPath
}

// run tests min functionality for apptainer run
func (c actionTests) actionRun(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name string
		argv []string
		exit int
	}{
		{
			name: "NoCommand",
			argv: []string{c.env.ImagePath},
			exit: 0,
		},
		{
			name: "True",
			argv: []string{c.env.ImagePath, "true"},
			exit: 0,
		},
		{
			name: "False",
			argv: []string{c.env.ImagePath, "false"},
			exit: 1,
		},
		{
			name: "ScifTestAppGood",
			argv: []string{"--app", "testapp", c.env.ImagePath},
			exit: 0,
		},
		{
			name: "ScifTestAppBad",
			argv: []string{"--app", "fakeapp", c.env.ImagePath},
			exit: 1,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("run"),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

// actionExec tests min functionality for apptainer exec
func (c actionTests) actionExec(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	user := e2e.UserProfile.HostUser(t)

	tests := []struct {
		name        string
		argv        []string
		exit        int
		wantOutputs []e2e.ApptainerCmdResultOp
	}{
		{
			name: "NoCommand",
			argv: []string{c.env.ImagePath},
			exit: 1,
		},
		{
			name: "True",
			argv: []string{c.env.ImagePath, "true"},
			exit: 0,
		},
		{
			name: "TrueAbsPAth",
			argv: []string{c.env.ImagePath, "/bin/true"},
			exit: 0,
		},
		{
			name: "False",
			argv: []string{c.env.ImagePath, "false"},
			exit: 1,
		},
		{
			name: "FalseAbsPath",
			argv: []string{c.env.ImagePath, "/bin/false"},
			exit: 1,
		},
		// Scif apps tests
		{
			name: "ScifTestAppGood",
			argv: []string{"--app", "testapp", c.env.ImagePath, "testapp.sh"},
			exit: 0,
		},
		{
			name: "ScifTestAppBad",
			argv: []string{"--app", "fakeapp", c.env.ImagePath, "testapp.sh"},
			exit: 1,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-d", "/scif"},
			exit: 0,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-d", "/scif/apps"},
			exit: 0,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-d", "/scif/data"},
			exit: 0,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-d", "/scif/apps/foo"},
			exit: 0,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-d", "/scif/apps/bar"},
			exit: 0,
		},
		// blocked by issue [scif-apps] Files created at install step fall into an unexpected path #2404
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-f", "/scif/apps/foo/filefoo.exec"},
			exit: 0,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-f", "/scif/apps/bar/filebar.exec"},
			exit: 0,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-d", "/scif/data/foo/output"},
			exit: 0,
		},
		{
			name: "ScifTestfolderOrg",
			argv: []string{c.env.ImagePath, "test", "-d", "/scif/data/foo/input"},
			exit: 0,
		},
		{
			name: "NoHome",
			argv: []string{"--no-home", c.env.ImagePath, "ls", "-ld", user.Dir},
			exit: 1,
		},
		// PID namespace, and override, in --containall mode. Uses --no-init to be able to check PID=1
		{
			name: "ContainAllPID",
			argv: []string{"--containall", "--no-init", c.env.ImagePath, "sh", "-c", "echo $$"},
			exit: 0,
			wantOutputs: []e2e.ApptainerCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "1"),
			},
		},
		{
			name: "ContainAllNoPID",
			argv: []string{"--containall", "--no-init", "--no-pid", c.env.ImagePath, "sh", "-c", "echo $$"},
			exit: 0,
			wantOutputs: []e2e.ApptainerCmdResultOp{
				e2e.ExpectOutput(e2e.UnwantedExactMatch, "1\n"),
			},
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("exec"),
			e2e.WithDir("/tmp"),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(tt.exit, tt.wantOutputs...),
		)
	}
}

// actionExecMultiProfile tests functionality using apptainer exec under all native profiles that do not involve user namespaces.
func (c actionTests) actionExecMultiProfile(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	for _, profile := range []e2e.Profile{e2e.RootProfile, e2e.FakerootProfile, e2e.UserProfile} {
		t.Run(profile.String(), func(t *testing.T) {
			// Create a temp testfile
			testdata, err := fs.MakeTmpDir(c.env.TestDir, "testdata", 0o755)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if !t.Failed() {
					os.RemoveAll(testdata)
				}
			})

			testdataTmp := filepath.Join(testdata, "tmp")
			if err := os.Mkdir(testdataTmp, 0o755); err != nil {
				t.Fatal(err)
			}

			// Create a temp testfile
			tmpfile, err := fs.MakeTmpFile(testdataTmp, "testApptainerExec.", 0o644)
			if err != nil {
				t.Fatal(err)
			}
			tmpfile.Close()

			basename := filepath.Base(tmpfile.Name())
			tmpfilePath := filepath.Join("/tmp", basename)
			vartmpfilePath := filepath.Join("/var/tmp", basename)
			homePath := filepath.Join("/home", basename)

			tests := []struct {
				name        string
				argv        []string
				exit        int
				wantOutputs []e2e.ApptainerCmdResultOp
			}{
				{
					name: "ContainOnly",
					argv: []string{"--contain", c.env.ImagePath, "test", "-f", tmpfilePath},
					exit: 1,
				},
				{
					name: "WorkdirOnly",
					argv: []string{"--workdir", testdata, c.env.ImagePath, "test", "-f", tmpfilePath},
					exit: 1,
				},
				{
					name: "WorkdirContain",
					argv: []string{"--workdir", testdata, "--contain", c.env.ImagePath, "test", "-f", tmpfilePath},
					exit: 0,
				},
				{
					name: "CwdGood",
					argv: []string{"--cwd", "/etc", c.env.ImagePath, "true"},
					exit: 0,
				},
				{
					name: "PwdGood",
					argv: []string{"--pwd", "/etc", c.env.ImagePath, "true"},
					exit: 0,
				},
				{
					name: "Home",
					argv: []string{"--home", testdata, c.env.ImagePath, "test", "-f", tmpfile.Name()},
					exit: 0,
				},
				{
					name: "HomePath",
					argv: []string{"--home", testdataTmp + ":/home", c.env.ImagePath, "test", "-f", homePath},
					exit: 0,
				},
				{
					name: "HomeTmp",
					argv: []string{"--home", "/tmp", c.env.ImagePath, "true"},
					exit: 0,
				},
				{
					name: "HomeTmpExplicit",
					argv: []string{"--home", "/tmp:/home", c.env.ImagePath, "true"},
					exit: 0,
				},
				{
					name: "UserBindTmp",
					argv: []string{"--bind", testdataTmp + ":/tmp", c.env.ImagePath, "test", "-f", tmpfilePath},
					exit: 0,
				},
				{
					name: "UserBindVarTmp",
					argv: []string{"--bind", testdataTmp + ":/var/tmp", c.env.ImagePath, "test", "-f", vartmpfilePath},
					exit: 0,
				},
			}

			for _, tt := range tests {
				c.env.RunApptainer(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(e2e.UserProfile),
					e2e.WithCommand("exec"),
					e2e.WithDir("/tmp"),
					e2e.WithArgs(tt.argv...),
					e2e.ExpectExit(tt.exit, tt.wantOutputs...),
				)
			}
		})
	}
}

// Shell interaction tests
func (c actionTests) actionShell(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	hostname, err := os.Hostname()
	err = errors.Wrap(err, "getting hostname")
	if err != nil {
		t.Fatalf("could not get hostname: %+v", err)
	}

	tests := []struct {
		name       string
		argv       []string
		consoleOps []e2e.ApptainerConsoleOp
		exit       int
	}{
		{
			name: "ShellExit",
			argv: []string{c.env.ImagePath},
			consoleOps: []e2e.ApptainerConsoleOp{
				// "cd /" to work around issue where a long
				// working directory name causes the test
				// to fail because the "Apptainer" that
				// we are looking for is chopped from the
				// front.
				// TODO(mem): This test was added back in 491a71716013654acb2276e4b37c2e015d2dfe09
				e2e.ConsoleSendLine("cd /"),
				e2e.ConsoleExpect("Singularity"),
				e2e.ConsoleSendLine("exit"),
			},
			exit: 0,
		},
		{
			name: "ShellHostname",
			argv: []string{c.env.ImagePath},
			consoleOps: []e2e.ApptainerConsoleOp{
				e2e.ConsoleSendLine("hostname"),
				e2e.ConsoleExpect(hostname),
				e2e.ConsoleSendLine("exit"),
			},
			exit: 0,
		},
		{
			name: "ShellBadCommand",
			argv: []string{c.env.ImagePath},
			consoleOps: []e2e.ApptainerConsoleOp{
				e2e.ConsoleSendLine("_a_fake_command"),
				e2e.ConsoleSendLine("exit"),
			},
			exit: 127,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("shell"),
			e2e.WithArgs(tt.argv...),
			e2e.ConsoleRun(tt.consoleOps...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

// STDPipe tests pipe stdin/stdout to apptainer actions cmd
func (c actionTests) STDPipe(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	stdinTests := []struct {
		name    string
		command string
		argv    []string
		input   string
		exit    int
	}{
		{
			name:    "TrueSTDIN",
			command: "exec",
			argv:    []string{c.env.ImagePath, "grep", "hi"},
			input:   "hi",
			exit:    0,
		},
		{
			name:    "FalseSTDIN",
			command: "exec",
			argv:    []string{c.env.ImagePath, "grep", "hi"},
			input:   "bye",
			exit:    1,
		},
		{
			name:    "TrueLibrary",
			command: "shell",
			argv:    []string{"oras://ghcr.io/apptainer/busybox:1.31.1"},
			input:   "true",
			exit:    0,
		},
		{
			name:    "FalseLibrary",
			command: "shell",
			argv:    []string{"oras://ghcr.io/apptainer/busybox:1.31.1"},
			input:   "false",
			exit:    1,
		},
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "TrueShub",
		// 	command: "shell",
		// 	argv:    []string{"shub://singularityhub/busybox"},
		// 	input:   "true",
		// 	exit:    0,
		// },
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "FalseShub",
		// 	command: "shell",
		// 	argv:    []string{"shub://singularityhub/busybox"},
		// 	input:   "false",
		// 	exit:    1,
		// },
	}

	var input bytes.Buffer

	for _, tt := range stdinTests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.argv...),
			e2e.WithStdin(&input),
			e2e.PreRun(func(_ *testing.T) {
				input.WriteString(tt.input)
			}),
			e2e.ExpectExit(tt.exit),
		)
		input.Reset()
	}

	user := e2e.CurrentUser(t)
	stdoutTests := []struct {
		name    string
		command string
		argv    []string
		output  string
		exit    int
	}{
		{
			name:    "AppsFoo",
			command: "run",
			argv:    []string{"--app", "foo", c.env.ImagePath},
			output:  "RUNNING FOO",
			exit:    0,
		},
		{
			name:    "CwdPath",
			command: "exec",
			argv:    []string{"--cwd", "/etc", c.env.ImagePath, "pwd"},
			output:  "/etc",
			exit:    0,
		},
		{
			name:    "PwdPath",
			command: "exec",
			argv:    []string{"--pwd", "/etc", c.env.ImagePath, "pwd"},
			output:  "/etc",
			exit:    0,
		},
		{
			name:    "Arguments",
			command: "run",
			argv:    []string{c.env.ImagePath, "foo"},
			output:  "Running command: foo",
			exit:    127,
		},
		{
			name:    "Permissions",
			command: "exec",
			argv:    []string{c.env.ImagePath, "id", "-un"},
			output:  user.Name,
			exit:    0,
		},
	}
	for _, tt := range stdoutTests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(
				tt.exit,
				e2e.ExpectOutput(e2e.ExactMatch, tt.output),
			),
		)
	}
}

// RunFromURI tests min functionality for apptainer run/exec URI://
func (c actionTests) RunFromURI(t *testing.T) {
	e2e.EnsureORASImage(t, c.env)

	runScript := "testdata/runscript.sh"
	bind := fmt.Sprintf("%s:/.singularity.d/runscript", runScript)

	fi, err := os.Stat(runScript)
	if err != nil {
		t.Fatalf("can't find %s", runScript)
	}
	size := strconv.Itoa(int(fi.Size()))

	tests := []struct {
		name    string
		command string
		argv    []string
		exit    int
		profile e2e.Profile
	}{
		// Run from supported URI's and check the runscript call works
		{
			name:    "RunFromLibraryOK",
			command: "run",
			argv:    []string{"--bind", bind, "oras://ghcr.io/apptainer/busybox:1.31.1", size},
			exit:    0,
			profile: e2e.UserProfile,
		},
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "RunFromShubOK",
		// 	command: "run",
		// 	argv:    []string{"--bind", bind, "shub://singularityhub/busybox", size},
		// 	exit:    0,
		// 	profile: e2e.UserProfile,
		// },
		{
			name:    "RunFromOrasOK",
			command: "run",
			argv:    []string{"--bind", bind, c.env.OrasTestImage, size},
			exit:    0,
			profile: e2e.UserProfile,
		},
		{
			name:    "RunFromLibraryKO",
			command: "run",
			argv:    []string{"--bind", bind, "oras://ghcr.io/apptainer/busybox:1.31.1", "0"},
			exit:    1,
			profile: e2e.UserProfile,
		},
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "RunFromShubKO",
		// 	command: "run",
		// 	argv:    []string{"--bind", bind, "shub://singularityhub/busybox", "0"},
		// 	exit:    1,
		// 	profile: e2e.UserProfile,
		// },
		{
			name:    "RunFromOrasKO",
			command: "run",
			argv:    []string{"--bind", bind, c.env.OrasTestImage, "0"},
			exit:    1,
			profile: e2e.UserProfile,
		},
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "ExecTrueShub",
		// 	command: "exec",
		// 	argv:    []string{"shub://singularityhub/busybox", "true"},
		// 	exit:    0,
		// 	profile: e2e.UserProfile,
		// },
		{
			name:    "ExecTrueOras",
			command: "exec",
			argv:    []string{c.env.OrasTestImage, "true"},
			exit:    0,
			profile: e2e.UserProfile,
		},
		{
			name:    "ExecFalseLibrary",
			command: "exec",
			argv:    []string{"oras://ghcr.io/apptainer/busybox:1.31.1", "false"},
			exit:    1,
			profile: e2e.UserProfile,
		},
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "ExecFalseShub",
		// 	command: "exec",
		// 	argv:    []string{"shub://singularityhub/busybox", "false"},
		// 	exit:    1,
		// 	profile: e2e.UserProfile,
		// },
		{
			name:    "ExecFalseOras",
			command: "exec",
			argv:    []string{c.env.OrasTestImage, "false"},
			exit:    1,
			profile: e2e.UserProfile,
		},

		// exec from URI with user namespace enabled
		{
			name:    "ExecTrueLibraryUserns",
			command: "exec",
			argv:    []string{"oras://ghcr.io/apptainer/busybox:1.31.1", "true"},
			exit:    0,
			profile: e2e.UserNamespaceProfile,
		},
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "ExecTrueShubUserns",
		// 	command: "exec",
		// 	argv:    []string{"shub://singularityhub/busybox", "true"},
		// 	exit:    0,
		// 	profile: e2e.UserNamespaceProfile,
		// },
		{
			name:    "ExecTrueOrasUserns",
			command: "exec",
			argv:    []string{c.env.OrasTestImage, "true"},
			exit:    0,
			profile: e2e.UserNamespaceProfile,
		},
		{
			name:    "ExecFalseLibraryUserns",
			command: "exec",
			argv:    []string{"oras://ghcr.io/apptainer/busybox:1.31.1", "false"},
			exit:    1,
			profile: e2e.UserNamespaceProfile,
		},
		// TODO(mem): re-enable this; disabled while shub is down
		// {
		// 	name:    "ExecFalseShubUserns",
		// 	command: "exec",
		// 	argv:    []string{"shub://singularityhub/busybox", "false"},
		// 	exit:    1,
		// 	profile: e2e.UserNamespaceProfile,
		// },
		{
			name:    "ExecFalseOrasUserns",
			command: "exec",
			argv:    []string{c.env.OrasTestImage, "false"},
			exit:    1,
			profile: e2e.UserNamespaceProfile,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

func (c actionTests) overlayCreate(t *testing.T, testdir string) string {
	// create overlay dir
	dir, err := os.MkdirTemp(testdir, "overlay-dir-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func (c actionTests) squashfsCreate(t *testing.T, testdir string) string {
	// squashfs img dir
	squashfsImage := filepath.Join(testdir, "squashfs.simg")
	// create root directory for squashfs image
	squashDir, err := os.MkdirTemp(testdir, "root-squash-dir-")
	if err != nil {
		t.Fatal(err)
	}
	squashMarkerFile := "squash_marker"
	if err := fs.Touch(filepath.Join(squashDir, squashMarkerFile)); err != nil {
		t.Fatal(err)
	}

	// create the squashfs overlay image
	cmd := exec.Command("mksquashfs", squashDir, squashfsImage, "-noappend", "-all-root")
	if res := cmd.Run(t); res.Error != nil {
		t.Fatalf("Unexpected error while running command.\n%s", res)
	}
	return squashfsImage
}

func (c actionTests) ext3Create(t *testing.T, testdir string) string {
	// create the ext3 overlay image
	ext3Img := filepath.Join(testdir, "ext3_fs.img")
	// create the overlay ext3 image
	cmd := exec.Command("dd", "if=/dev/zero", "of="+ext3Img, "bs=1M", "count=64", "status=none")
	if res := cmd.Run(t); res.Error != nil {
		t.Fatalf("Unexpected error while running command.\n%s", res)
	}
	cmd = exec.Command("mkfs.ext3", "-q", "-F", ext3Img)
	if res := cmd.Run(t); res.Error != nil {
		t.Fatalf("Unexpected error while running command.\n%s", res)
	}
	return ext3Img
}

func (c actionTests) sandboxCreate(t *testing.T, testdir string) string {
	// create the sandbox dir
	sandboxImage := filepath.Join(testdir, "sandbox")
	// create a sandbox image from test image
	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--force", "--sandbox", sandboxImage, c.env.ImagePath),
		e2e.PostRun(func(t *testing.T) {
			if t.Failed() {
				t.Fatalf("failed to create sandbox %s from test image %s", sandboxImage, c.env.ImagePath)
			}
		}),
		e2e.ExpectExit(0),
	)
	return sandboxImage
}

func (c actionTests) embeddedSifCreate(t *testing.T, testdir string) string {
	// create overlay embedded img
	embeddedSif := filepath.Join(testdir, "embedded.sif")
	err := fs.CopyFile(c.env.ImagePath, embeddedSif, 0o700)
	if err != nil {
		t.Fatalf("Failed to copy file from %s to %s", c.env.ImagePath, embeddedSif)
	}

	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("overlay"),
		e2e.WithArgs("create", embeddedSif),
		e2e.ExpectExit(0),
	)

	return embeddedSif
}

func (c actionTests) PersistentOverlay(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	require.Filesystem(t, "overlay")

	require.Command(t, "mkfs.ext3")
	require.Command(t, "mksquashfs")
	require.Command(t, "dd")

	// the order should not be changed, it'll cause permission denied error. RootProfile will create
	// overlay folder with the permission other users can not access.
	profiles := []e2e.Profile{e2e.UserProfile, e2e.UserNamespaceProfile, e2e.RootProfile}

	// create the test dir
	testdir, err := os.MkdirTemp(c.env.TestDir, "persitent-overlay-")
	if err != nil {
		t.Fatal(err)
	}

	defer e2e.Privileged(func(t *testing.T) {
		if t.Failed() {
			t.Logf("Not removing directory %s for test %s", testdir, t.Name())
			return
		}
		err := os.RemoveAll(testdir)
		if err != nil {
			t.Logf("Error while removing directory %s for test %s: %#v", testdir, t.Name(), err)
		}
	})(t)

	dir := c.overlayCreate(t, testdir)
	squashfsImage := c.squashfsCreate(t, testdir)
	ext3Img := c.ext3Create(t, testdir)
	sandboxImage := c.sandboxCreate(t, testdir)
	embeddedSif := c.embeddedSifCreate(t, testdir)

	tests := []struct {
		name     string
		argv     []string
		fakeroot bool
		exit     int
		dir      string
	}{
		{
			name:     "overlay_create",
			argv:     []string{"--overlay", dir, c.env.ImagePath, "touch", "/dir_overlay"},
			fakeroot: true,
			exit:     0,
		},
		{
			name:     "overlay_ext3_create",
			argv:     []string{"--overlay", ext3Img, c.env.ImagePath, "touch", "/ext3_overlay"},
			fakeroot: true,
			exit:     0,
		},
		{
			name:     "overlay_multiple_create",
			argv:     []string{"--overlay", ext3Img, "--overlay", squashfsImage + ":ro", c.env.ImagePath, "touch", "/multiple_overlay_fs"},
			fakeroot: true,
			exit:     0,
		},
		{
			name:     "overlay_find",
			argv:     []string{"--overlay", dir, c.env.ImagePath, "test", "-f", "/dir_overlay"},
			fakeroot: true,
			exit:     0,
		},
		{
			name: "overlay_find_with_writable_fail",
			argv: []string{"--overlay", dir, "--writable", c.env.ImagePath, "true"},
			exit: 255,
		},
		{
			name: "overlay_find_with_writable_tmpfs",
			argv: []string{"--overlay", dir + ":ro", "--writable-tmpfs", c.env.ImagePath, "test", "-f", "/dir_overlay"},
			exit: 0,
		},
		{
			name: "overlay_find_with_writable_tmpfs_fail",
			argv: []string{"--overlay", dir, "--writable-tmpfs", c.env.ImagePath, "true"},
			exit: 255,
		},
		{
			name:     "overlay_ext3_find",
			argv:     []string{"--overlay", ext3Img, c.env.ImagePath, "test", "-f", "/ext3_overlay"},
			fakeroot: true,
			exit:     0,
		},
		{
			name: "overlay_multiple_writable_fail",
			argv: []string{"--overlay", ext3Img, "--overlay", ext3Img, c.env.ImagePath, "true"},
			exit: 255,
		},
		{
			name: "overlay_squashFS_find",
			argv: []string{"--overlay", squashfsImage + ":ro", c.env.ImagePath, "test", "-f", "/squash_marker"},
			exit: 0,
		},
		{
			name: "overlay_squashFS_find_without_ro",
			argv: []string{"--overlay", squashfsImage, c.env.ImagePath, "test", "-f", "/squash_marker"},
			exit: 0,
		},
		{
			name:     "overlay_multiple_find_ext3",
			argv:     []string{"--overlay", ext3Img, "--overlay", squashfsImage + ":ro", c.env.ImagePath, "test", "-f", "/multiple_overlay_fs"},
			fakeroot: true,
			exit:     0,
		},
		{
			name:     "overlay_multiple_find_squashfs",
			argv:     []string{"--overlay", ext3Img, "--overlay", squashfsImage + ":ro", c.env.ImagePath, "test", "-f", "/squash_marker"},
			fakeroot: true,
			exit:     0,
		},
		{
			name:     "overlay_noroot",
			argv:     []string{"--overlay", dir, c.env.ImagePath, "true"},
			fakeroot: true,
			exit:     0,
		},
		{
			name: "overlay_noflag",
			argv: []string{c.env.ImagePath, "test", "-f", "/foo_overlay"},
			exit: 1,
		},
		{
			// https://github.com/apptainer/singularity/issues/4329
			name: "SIF_writable_without_overlay_partition_issue_4329",
			argv: []string{"--writable", c.env.ImagePath, "true"},
			exit: 255,
		},
		{
			// https://github.com/apptainer/singularity/issues/4270
			name:     "overlay_dir_relative_path_issue_4270",
			argv:     []string{"--overlay", filepath.Base(dir), sandboxImage, "test", "-f", "/dir_overlay"},
			dir:      filepath.Dir(dir),
			fakeroot: true,
			exit:     0,
		},
		{
			name: "Embedded overlay partition in SIF",
			argv: []string{embeddedSif, "ps"},
			exit: 0,
		},
	}

	for _, profile := range profiles {
		for _, tt := range tests {
			var args []string
			if (profile.String() == "User" || profile.String() == "UserNamespace") && tt.fakeroot {
				args = append(args, "--fakeroot")
			}
			args = append(args, tt.argv...)

			c.env.RunApptainer(
				t,
				e2e.AsSubtest(tt.name+"_"+profile.String()),
				e2e.WithProfile(profile),
				e2e.WithDir(tt.dir),
				e2e.WithCommand("exec"),
				e2e.WithArgs(args...),
				e2e.ExpectExit(tt.exit),
			)
		}
	}
}

func (c actionTests) actionBasicProfiles(t *testing.T) {
	env := c.env

	e2e.EnsureImage(t, env)

	tests := []struct {
		name    string
		command string
		argv    []string
		exit    int
	}{
		{
			name:    "ExecTrue",
			command: "exec",
			argv:    []string{env.ImagePath, "true"},
			exit:    0,
		},
		{
			name:    "ExecPidNsTrue",
			command: "exec",
			argv:    []string{"--pid", env.ImagePath, "true"},
			exit:    0,
		},
		{
			name:    "ExecFalse",
			command: "exec",
			argv:    []string{env.ImagePath, "false"},
			exit:    1,
		},
		{
			name:    "ExecPidNsFalse",
			command: "exec",
			argv:    []string{"--pid", env.ImagePath, "false"},
			exit:    1,
		},
		{
			name:    "RunTrue",
			command: "run",
			argv:    []string{env.ImagePath, "true"},
			exit:    0,
		},
		{
			name:    "RunPidNsTrue",
			command: "run",
			argv:    []string{"--pid", env.ImagePath, "true"},
			exit:    0,
		},
		{
			name:    "RunFalse",
			command: "run",
			argv:    []string{env.ImagePath, "false"},
			exit:    1,
		},
		{
			name:    "RunPidNsFalse",
			command: "run",
			argv:    []string{"--pid", env.ImagePath, "false"},
			exit:    1,
		},
		{
			name:    "RunBindTrue",
			command: "run",
			argv:    []string{"--bind", "/etc/passwd", env.ImagePath, "true"},
			exit:    0,
		},
		{
			name:    "RunBindFalse",
			command: "run",
			argv:    []string{"--bind", "/etc/passwd", env.ImagePath, "false"},
			exit:    1,
		},
	}

	for _, profile := range e2e.Profiles {
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				env.RunApptainer(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand(tt.command),
					e2e.WithArgs(tt.argv...),
					e2e.ExpectExit(tt.exit),
				)
			}
		})
	}
}

func (c actionTests) actionNetwork(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	e2e.Privileged(require.Network)(t)

	tests := []struct {
		name       string
		profile    e2e.Profile
		netType    string
		expectExit int
	}{
		{
			name:       "BridgeNetwork",
			profile:    e2e.RootProfile,
			netType:    "bridge",
			expectExit: 0,
		},
		{
			name:       "PtpNetwork",
			profile:    e2e.RootProfile,
			netType:    "ptp",
			expectExit: 0,
		},
		{
			name:       "UnknownNetwork",
			profile:    e2e.RootProfile,
			netType:    "unknown",
			expectExit: 255,
		},
		{
			name:       "FakerootNetwork",
			profile:    e2e.FakerootProfile,
			netType:    "fakeroot",
			expectExit: 0,
		},
		{
			name:       "NoneNetwork",
			profile:    e2e.UserProfile,
			netType:    "none",
			expectExit: 0,
		},
		{
			name:       "UserBridgeNetwork",
			profile:    e2e.UserProfile,
			netType:    "bridge",
			expectExit: 255,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--net", "--network", tt.netType, c.env.ImagePath, "id"),
			e2e.ExpectExit(tt.expectExit),
		)
	}
}

func (c actionTests) actionNetnsPath(t *testing.T) {
	e2e.EnsureImage(t, c.env)
	require.Command(t, "ip")

	nsName := "apptainer-e2e"
	nsPath := filepath.Join("/run", "netns", nsName)

	netnsInode := uint64(0)

	e2e.Privileged(func(t *testing.T) {
		t.Log("Creating netns")
		cmd := exec.Command("ip", "netns", "add", nsName)
		if res := cmd.Run(t); res.Error != nil {
			t.Fatalf("While creating network namespace: %s", res)
		}
		fi, err := os.Stat(nsPath)
		if err != nil {
			t.Fatal(err)
		}
		stat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatal("Stat_t assertion error")
		}
		netnsInode = stat.Ino
		t.Logf("Netns inode: %d", netnsInode)
	})(t)

	defer e2e.Privileged(func(t *testing.T) {
		t.Log("Deleting netns")
		cmd := exec.Command("ip", "netns", "delete", nsName)
		if res := cmd.Run(t); res.Error != nil {
			t.Fatalf("While deleting network namespace: %s", res)
		}
	})(t)

	tests := []struct {
		name         string
		profile      e2e.Profile
		netnsPath    string
		expectExit   int
		expectOutput string
	}{
		{
			name:         "root",
			profile:      e2e.RootProfile,
			netnsPath:    nsPath,
			expectExit:   0,
			expectOutput: fmt.Sprintf("%d", netnsInode),
		},
		{
			name:       "user",
			profile:    e2e.UserProfile,
			netnsPath:  nsPath,
			expectExit: 255,
		},
		{
			name:       "userns",
			profile:    e2e.UserNamespaceProfile,
			netnsPath:  nsPath,
			expectExit: 255,
		},
		{
			name:       "fakeroot",
			profile:    e2e.FakerootProfile,
			netnsPath:  nsPath,
			expectExit: 255,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--netns-path", tt.netnsPath, c.env.ImagePath, "stat", "-L", "-c", "%i", "/proc/self/ns/net"),
			e2e.ExpectExit(tt.expectExit, e2e.ExpectOutput(e2e.ContainMatch, tt.expectOutput)),
		)
	}
}

//nolint:maintidx
func (c actionTests) actionBinds(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	workspace, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "bind-workspace-", "")
	sandbox, _ := e2e.MakeTempDir(t, workspace, "sandbox-", "")
	defer e2e.Privileged(cleanup)(t)

	contCanaryDir := "/canary"
	hostCanaryDir := filepath.Join(workspace, "canary")

	contCanaryFile := "/canary/file"
	hostCanaryFile := filepath.Join(hostCanaryDir, "file")
	hostCanaryFileWithComma := filepath.Join(hostCanaryDir, "file,comma")
	hostCanaryFileWithColon := filepath.Join(hostCanaryDir, "file:colon")

	canaryFileBind := hostCanaryFile + ":" + contCanaryFile
	canaryFileMount := "type=bind,source=" + hostCanaryFile + ",destination=" + contCanaryFile
	canaryDirBind := hostCanaryDir + ":" + contCanaryDir
	canaryDirMount := "type=bind,source=" + hostCanaryDir + ",destination=" + contCanaryDir

	hostHomeDir := filepath.Join(workspace, "home")
	hostWorkDir := filepath.Join(workspace, "workdir")

	createWorkspaceDirs := func(t *testing.T) {
		e2e.Privileged(func(t *testing.T) {
			if err := os.RemoveAll(hostCanaryDir); err != nil && !os.IsNotExist(err) {
				t.Fatalf("failed to delete canary_dir: %s", err)
			}
			if err := os.RemoveAll(hostHomeDir); err != nil && !os.IsNotExist(err) {
				t.Fatalf("failed to delete workspace home: %s", err)
			}
			if err := os.RemoveAll(hostWorkDir); err != nil && !os.IsNotExist(err) {
				t.Fatalf("failed to delete workspace work: %s", err)
			}
		})(t)

		if err := fs.Mkdir(hostCanaryDir, 0o777); err != nil {
			t.Fatalf("failed to create canary_dir: %s", err)
		}
		if err := fs.Touch(hostCanaryFile); err != nil {
			t.Fatalf("failed to create canary_file: %s", err)
		}
		if err := fs.Touch(hostCanaryFileWithComma); err != nil {
			t.Fatalf("failed to create canary_file_comma: %s", err)
		}
		if err := fs.Touch(hostCanaryFileWithColon); err != nil {
			t.Fatalf("failed to create canary_file_colon: %s", err)
		}
		if err := os.Chmod(hostCanaryFile, 0o777); err != nil {
			t.Fatalf("failed to apply permissions on canary_file: %s", err)
		}
		if err := fs.Mkdir(hostHomeDir, 0o777); err != nil {
			t.Fatalf("failed to create workspace home directory: %s", err)
		}
		if err := fs.Mkdir(hostWorkDir, 0o777); err != nil {
			t.Fatalf("failed to create workspace work directory: %s", err)
		}
	}

	// convert test image to sandbox
	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--force", "--sandbox", sandbox, c.env.ImagePath),
		e2e.ExpectExit(0),
	)

	checkHostFn := func(path string, fn func(string) bool) func(*testing.T) {
		return func(t *testing.T) {
			if t.Failed() {
				return
			}
			if !fn(path) {
				t.Errorf("%s not found on host", path)
			}
			if err := os.RemoveAll(path); err != nil {
				t.Errorf("failed to delete %s: %s", path, err)
			}
		}
	}
	checkHostFile := func(path string) func(*testing.T) {
		return checkHostFn(path, fs.IsFile)
	}
	checkHostDir := func(path string) func(*testing.T) {
		return checkHostFn(path, fs.IsDir)
	}

	tests := []struct {
		name    string
		args    []string
		postRun func(*testing.T)
		exit    int
	}{
		{
			name: "NonExistentSource",
			args: []string{
				"--bind", "/non/existent/source/path",
				sandbox,
				"true",
			},
			exit: 255,
		},
		{
			name: "RelativeBindDestination",
			args: []string{
				"--bind", hostCanaryFile + ":relative",
				sandbox,
				"true",
			},
			exit: 255,
		},
		{
			name: "SimpleFile",
			args: []string{
				"--bind", canaryFileBind,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleFileUnderlay",
			args: []string{
				"--underlay",
				"--bind", canaryFileBind,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleFileCwd",
			args: []string{
				"--bind", canaryFileBind,
				"--cwd", contCanaryDir,
				sandbox,
				"test", "-f", "file",
			},
			exit: 0,
		},
		{
			name: "SimpleFilePwd",
			args: []string{
				"--bind", canaryFileBind,
				"--pwd", contCanaryDir,
				sandbox,
				"test", "-f", "file",
			},
			exit: 0,
		},
		{
			name: "SimpleFilePwdUnderlay",
			args: []string{
				"--underlay",
				"--bind", canaryFileBind,
				"--pwd", contCanaryDir,
				sandbox,
				"test", "-f", "file",
			},
			exit: 0,
		},
		{
			name: "SimpleDir",
			args: []string{
				"--bind", canaryDirBind,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleDirCwd",
			args: []string{
				"--bind", canaryDirBind,
				"--cwd", contCanaryDir,
				sandbox,
				"test", "-f", "file",
			},
			exit: 0,
		},
		{
			name: "SimpleDirPwd",
			args: []string{
				"--bind", canaryDirBind,
				"--pwd", contCanaryDir,
				sandbox,
				"test", "-f", "file",
			},
			exit: 0,
		},
		{
			name: "SimpleDirUnderlay",
			args: []string{
				"--underlay",
				"--bind", canaryDirBind,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleDirPwdUnderlay",
			args: []string{
				"--underlay",
				"--bind", canaryDirBind,
				"--pwd", contCanaryDir,
				sandbox,
				"test", "-f", "file",
			},
			exit: 0,
		},
		{
			name: "SimpleFileWritableOK",
			args: []string{
				"--writable",
				"--bind", hostCanaryFile,
				sandbox,
				"test", "-f", hostCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleFileWritableUnderlayOK",
			args: []string{
				"--underlay",
				"--writable",
				"--bind", hostCanaryFile,
				sandbox,
				"test", "-f", hostCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleFileWritableKO",
			args: []string{
				"--writable",
				"--bind", canaryFileBind,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 255,
		},
		{
			name: "SimpleDirWritableOK",
			args: []string{
				"--writable",
				"--bind", hostCanaryDir,
				sandbox,
				"test", "-f", hostCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleDirWritableKO",
			args: []string{
				"--writable",
				"--bind", canaryDirBind,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 255,
		},
		{
			name: "HomeContainOverride",
			args: []string{
				"--contain",
				"--bind", hostCanaryDir + ":/home",
				sandbox,
				"test", "-f", "/home/file",
			},
			exit: 0,
		},
		{
			name: "HomeContainOverrideUnderlay",
			args: []string{
				"--underlay",
				"--contain",
				"--bind", hostCanaryDir + ":/home",
				sandbox,
				"test", "-f", "/home/file",
			},
			exit: 0,
		},
		{
			name: "TmpContainOverride",
			args: []string{
				"--contain",
				"--bind", hostCanaryDir + ":/tmp",
				sandbox,
				"test", "-f", "/tmp/file",
			},
			exit: 0,
		},
		{
			name: "TmpContainOverrideUnderlay",
			args: []string{
				"--underlay",
				"--contain",
				"--bind", hostCanaryDir + ":/tmp",
				sandbox,
				"test", "-f", "/tmp/file",
			},
			exit: 0,
		},
		{
			name: "VarTmpContainOverride",
			args: []string{
				"--contain",
				"--bind", hostCanaryDir + ":/var/tmp",
				sandbox,
				"test", "-f", "/var/tmp/file",
			},
			exit: 0,
		},
		{
			name: "VarTmpContainOverrideUnderlay",
			args: []string{
				"--underlay",
				"--contain",
				"--bind", hostCanaryDir + ":/var/tmp",
				sandbox,
				"test", "-f", "/var/tmp/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkOneLevelFileBind",
			args: []string{
				"--bind", hostCanaryFile + ":/var/etc/symlink1",
				sandbox,
				"test", "-f", "/etc/symlink1",
			},
			exit: 0,
		},
		{
			name: "SymlinkOneLevelFileBindUnderlay",
			args: []string{
				"--underlay",
				"--bind", hostCanaryFile + ":/var/etc/symlink1",
				sandbox,
				"test", "-f", "/etc/symlink1",
			},
			exit: 0,
		},
		{
			name: "SymlinkTwoLevelFileBind",
			args: []string{
				"--bind", hostCanaryFile + ":/var/etc/madness/symlink2",
				sandbox,
				"test", "-f", "/madness/symlink2",
			},
			exit: 0,
		},
		{
			name: "SymlinkTwoLevelFileBindUnderlay",
			args: []string{
				"--underlay",
				"--bind", hostCanaryFile + ":/var/etc/madness/symlink2",
				sandbox,
				"test", "-f", "/madness/symlink2",
			},
			exit: 0,
		},
		{
			name: "SymlinkOneLevelDirBind",
			args: []string{
				"--bind", hostCanaryDir + ":/var/etc",
				sandbox,
				"test", "-f", "/etc/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkOneLevelDirBindUnderlay",
			args: []string{
				"--underlay",
				"--bind", hostCanaryDir + ":/var/etc",
				sandbox,
				"test", "-f", "/etc/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkTwoLevelDirBind",
			args: []string{
				"--bind", hostCanaryDir + ":/var/etc/madness",
				sandbox,
				"test", "-f", "/madness/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkTwoLevelDirBindUnderlay",
			args: []string{
				"--underlay",
				"--bind", hostCanaryDir + ":/var/etc/madness",
				sandbox,
				"test", "-f", "/madness/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkOneLevelNewDirBind",
			args: []string{
				"--bind", hostCanaryDir + ":/var/etc/new",
				sandbox,
				"test", "-f", "/etc/new/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkOneLevelNewDirBindUnderlay",
			args: []string{
				"--underlay",
				"--bind", hostCanaryDir + ":/var/etc/new",
				sandbox,
				"test", "-f", "/etc/new/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkTwoLevelNewDirBind",
			args: []string{
				"--bind", hostCanaryDir + ":/var/etc/madness/new",
				sandbox,
				"test", "-f", "/madness/new/file",
			},
			exit: 0,
		},
		{
			name: "SymlinkTwoLevelNewDirBindUnderlay",
			args: []string{
				"--underlay",
				"--bind", hostCanaryDir + ":/var/etc/madness/new",
				sandbox,
				"test", "-f", "/madness/new/file",
			},
			exit: 0,
		},
		{
			name: "NestedBindFile",
			args: []string{
				"--bind", canaryDirBind,
				"--bind", hostCanaryFile + ":" + filepath.Join(contCanaryDir, "file2"),
				sandbox,
				"test", "-f", "/canary/file2",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "file2")),
			exit:    0,
		},
		{
			name: "NestedBindFileUnderlay",
			args: []string{
				"--underlay",
				"--bind", canaryDirBind,
				"--bind", hostCanaryFile + ":" + filepath.Join(contCanaryDir, "file2"),
				sandbox,
				"test", "-f", "/canary/file2",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "file2")),
			exit:    0,
		},
		{
			name: "NestedBindDir",
			args: []string{
				"--bind", canaryDirBind,
				"--bind", hostCanaryDir + ":" + filepath.Join(contCanaryDir, "dir2"),
				sandbox,
				"test", "-d", "/canary/dir2",
			},
			postRun: checkHostDir(filepath.Join(hostCanaryDir, "dir2")),
			exit:    0,
		},
		{
			name: "NestedBindDirUnderlay",
			args: []string{
				"--underlay",
				"--bind", canaryDirBind,
				"--bind", hostCanaryDir + ":" + filepath.Join(contCanaryDir, "dir2"),
				sandbox,
				"test", "-d", "/canary/dir2",
			},
			postRun: checkHostDir(filepath.Join(hostCanaryDir, "dir2")),
			exit:    0,
		},
		{
			name: "MultipleNestedBindDir",
			args: []string{
				"--bind", canaryDirBind,
				"--bind", hostCanaryDir + ":" + filepath.Join(contCanaryDir, "dir2"),
				"--bind", hostCanaryFile + ":" + filepath.Join(filepath.Join(contCanaryDir, "dir2"), "nested"),
				sandbox,
				"test", "-f", "/canary/dir2/nested",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "nested")),
			exit:    0,
		},
		{
			name: "MultipleNestedBindDirUnderlay",
			args: []string{
				"--underlay",
				"--bind", canaryDirBind,
				"--bind", hostCanaryDir + ":" + filepath.Join(contCanaryDir, "dir2"),
				"--bind", hostCanaryFile + ":" + filepath.Join(filepath.Join(contCanaryDir, "dir2"), "nested"),
				sandbox,
				"test", "-f", "/canary/dir2/nested",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "nested")),
			exit:    0,
		},
		{
			name: "CustomHomeOneToOne",
			args: []string{
				"--home", hostHomeDir,
				"--bind", hostCanaryDir + ":" + filepath.Join(hostHomeDir, "canary121RO"),
				sandbox,
				"test", "-f", filepath.Join(hostHomeDir, "canary121RO/file"),
			},
			postRun: checkHostDir(filepath.Join(hostHomeDir, "canary121RO")),
			exit:    0,
		},
		{
			name: "CustomHomeOneToOneUnderlay",
			args: []string{
				"--underlay",
				"--home", hostHomeDir,
				"--bind", hostCanaryDir + ":" + filepath.Join(hostHomeDir, "canary121RO"),
				sandbox,
				"test", "-f", filepath.Join(hostHomeDir, "canary121RO/file"),
			},
			postRun: checkHostDir(filepath.Join(hostHomeDir, "canary121RO")),
			exit:    0,
		},
		{
			name: "CustomHomeBind",
			args: []string{
				"--home", hostHomeDir + ":/home/e2e",
				"--bind", hostCanaryDir + ":/home/e2e/canaryRO",
				sandbox,
				"test", "-f", "/home/e2e/canaryRO/file",
			},
			postRun: checkHostDir(filepath.Join(hostHomeDir, "canaryRO")),
			exit:    0,
		},
		{
			name: "CustomHomeBindUnderlay",
			args: []string{
				"--underlay",
				"--home", hostHomeDir + ":/home/e2e",
				"--bind", hostCanaryDir + ":/home/e2e/canaryRO",
				sandbox,
				"test", "-f", "/home/e2e/canaryRO/file",
			},
			postRun: checkHostDir(filepath.Join(hostHomeDir, "canaryRO")),
			exit:    0,
		},
		{
			name: "CustomHomeBindWritableOK",
			args: []string{
				"--home", hostHomeDir + ":/home/e2e",
				"--bind", hostCanaryDir + ":/home/e2e/canaryRW",
				"--writable",
				sandbox,
				"test", "-f", "/home/e2e/canaryRW/file",
			},
			postRun: checkHostDir(filepath.Join(hostHomeDir, "canaryRW")),
			exit:    0,
		},
		{
			name: "CustomHomeBindWritableKO",
			args: []string{
				"--home", canaryDirBind,
				"--writable",
				sandbox,
				"true",
			},
			exit: 255,
		},
		{
			name: "WorkdirTmpBind",
			args: []string{
				"--workdir", hostWorkDir,
				"--contain",
				"--bind", hostCanaryDir + ":/tmp/canary/dir",
				sandbox,
				"test", "-f", "/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "WorkdirTmpBindUnderlay",
			args: []string{
				"--underlay",
				"--workdir", hostWorkDir,
				"--contain",
				"--bind", hostCanaryDir + ":/tmp/canary/dir",
				sandbox,
				"test", "-f", "/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "WorkdirTmpBindWritable",
			args: []string{
				"--writable",
				"--workdir", hostWorkDir,
				"--contain",
				"--bind", hostCanaryDir + ":/tmp/canary/dir",
				sandbox,
				"test", "-f", "/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "WorkdirVarTmpBind",
			args: []string{
				"--workdir", hostWorkDir,
				"--contain",
				"--bind", hostCanaryDir + ":/var/tmp/canary/dir",
				sandbox,
				"test", "-f", "/var/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "var_tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "WorkdirVarTmpBindUnderlay",
			args: []string{
				"--underlay",
				"--workdir", hostWorkDir,
				"--contain",
				"--bind", hostCanaryDir + ":/var/tmp/canary/dir",
				sandbox,
				"test", "-f", "/var/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "var_tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "WorkdirVarTmpBindWritable",
			args: []string{
				"--writable",
				"--workdir", hostWorkDir,
				"--contain",
				"--bind", hostCanaryDir + ":/var/tmp/canary/dir",
				sandbox,
				"test", "-f", "/var/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "var_tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "ScratchTmpfsBind",
			args: []string{
				"--scratch", "/scratch",
				"--bind", hostCanaryDir + ":/scratch/dir",
				sandbox,
				"test", "-f", "/scratch/dir/file",
			},
			exit: 0,
		},
		{
			name: "ScratchTmpfsBindUnderlay",
			args: []string{
				"--underlay",
				"--scratch", "/scratch",
				"--bind", hostCanaryDir + ":/scratch/dir",
				sandbox,
				"test", "-f", "/scratch/dir/file",
			},
			exit: 0,
		},
		{
			name: "ScratchWorkdirBind",
			args: []string{
				"--workdir", hostWorkDir,
				"--scratch", "/scratch",
				"--bind", hostCanaryDir + ":/scratch/dir",
				sandbox,
				"test", "-f", "/scratch/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "scratch/scratch", "dir")),
			exit:    0,
		},
		{
			name: "ScratchWorkdirBindUnderlay",
			args: []string{
				"--underlay",
				"--workdir", hostWorkDir,
				"--scratch", "/scratch",
				"--bind", hostCanaryDir + ":/scratch/dir",
				sandbox,
				"test", "-f", "/scratch/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "scratch/scratch", "dir")),
			exit:    0,
		},
		{
			name: "BindFileWithCommaOK",
			args: []string{
				"--bind", strings.ReplaceAll(hostCanaryFileWithComma, ",", "\\,") + ":" + contCanaryFile,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "BindFileWithCommaUnderlayOK",
			args: []string{
				"--underlay",
				"--bind", strings.ReplaceAll(hostCanaryFileWithComma, ",", "\\,") + ":" + contCanaryFile,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "BindFileWithCommaKO",
			args: []string{
				"--bind", hostCanaryFileWithComma + ":" + contCanaryFile,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 255,
		},
		{
			name: "BindFileWithColonOK",
			args: []string{
				"--bind", strings.ReplaceAll(hostCanaryFileWithColon, ":", "\\:") + ":" + contCanaryFile,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "BindFileWithColonKO",
			args: []string{
				"--bind", hostCanaryFileWithColon + ":" + contCanaryFile,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 255,
		},
		// For the --mount variants we are really just verifying the CLI
		// acceptance of one or more --mount flags. Translation from --mount
		// strings to BindPath structs is checked in unit tests. The
		// functionality of bind mounts of various kinds is already checked
		// above, with --bind flags. No need to duplicate all of these.
		{
			name: "MountSingle",
			args: []string{
				"--mount", canaryFileMount,
				sandbox,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "MountNested",
			args: []string{
				"--mount", canaryDirMount,
				"--mount", "source=" + hostCanaryFile + ",destination=" + filepath.Join(contCanaryDir, "file3"),
				sandbox,
				"test", "-f", "/canary/file3",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "file3")),
			exit:    0,
		},
	}

	for _, profile := range e2e.Profiles {
		createWorkspaceDirs(t)

		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				c.env.RunApptainer(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand("exec"),
					e2e.WithArgs(tt.args...),
					e2e.PostRun(tt.postRun),
					e2e.ExpectExit(tt.exit),
				)
			}
		})
	}
}

func (c actionTests) actionLayerType(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	sandbox, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "action-layertype-sandbox", "")
	defer e2e.Privileged(cleanup)(t)

	// convert test image to sandbox
	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--force", "--sandbox", sandbox, c.env.ImagePath),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name       string
		profile    e2e.Profile
		args       []string
		expectType string
	}{
		{
			name:    "RootDefault",
			profile: e2e.RootProfile,
			args: []string{
				c.env.ImagePath,
			},
			expectType: "overlay",
		},
		{
			name:    "RootWithUnderlayOption",
			profile: e2e.RootProfile,
			args: []string{
				"--underlay",
				c.env.ImagePath,
			},
			expectType: "tmpfs",
		},
		{
			name:    "UserDefault",
			profile: e2e.UserProfile,
			args: []string{
				c.env.ImagePath,
			},
			expectType: "overlay",
		},
		{
			name:    "UserWithUnderlayOption",
			profile: e2e.UserProfile,
			args: []string{
				"--underlay",
				c.env.ImagePath,
			},
			expectType: "tmpfs",
		},
		{
			name:    "UserSandboxDefault",
			profile: e2e.UserProfile,
			args: []string{
				sandbox,
			},
			expectType: "overlay",
		},
		{
			name:    "UserSandboxWithUnderlayOption",
			profile: e2e.UserProfile,
			args: []string{
				"--underlay",
				sandbox,
			},
			expectType: "tmpfs",
		},
		{
			name:    "UserNamespaceDefault",
			profile: e2e.UserNamespaceProfile,
			args: []string{
				c.env.ImagePath,
			},
			expectType: "overlay",
		},
		{
			name:    "UserNamespaceWithUnderlayOption",
			profile: e2e.UserNamespaceProfile,
			args: []string{
				"--underlay",
				c.env.ImagePath,
			},
			expectType: "tmpfs",
		},
		{
			name:    "UserNamespaceSandboxDefault",
			profile: e2e.UserNamespaceProfile,
			args: []string{
				sandbox,
			},
			expectType: "overlay",
		},
		{
			name:    "UserNamespaceSandboxWithUnderlayOption",
			profile: e2e.UserNamespaceProfile,
			args: []string{
				"--underlay",
				sandbox,
			},
			expectType: "tmpfs",
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(append(tt.args, "mount")...),
			e2e.ExpectExit(0, e2e.ExpectOutput(e2e.ContainMatch, "on / type "+tt.expectType)),
		)
	}
}

func (c actionTests) exitSignals(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name string
		args []string
		exit int
	}{
		{
			name: "Exit0",
			args: []string{c.env.ImagePath, "/bin/sh", "-c", "exit 0"},
			exit: 0,
		},
		{
			name: "Exit1",
			args: []string{c.env.ImagePath, "/bin/sh", "-c", "exit 1"},
			exit: 1,
		},
		{
			name: "Exit134",
			args: []string{c.env.ImagePath, "/bin/sh", "-c", "exit 134"},
			exit: 134,
		},
		{
			name: "SignalKill",
			args: []string{c.env.ImagePath, "/bin/sh", "-c", "kill -KILL $$"},
			exit: 137,
		},
		{
			name: "SignalAbort",
			args: []string{c.env.ImagePath, "/bin/sh", "-c", "kill -ABRT $$"},
			exit: 134,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

func (c actionTests) fuseMount(t *testing.T) {
	t.Skip("Broken on github for ubuntu 18.04/20.04")

	require.Filesystem(t, "fuse")

	u := e2e.UserProfile.HostUser(t)

	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "sshfs-", "")
	defer e2e.Privileged(cleanup)(t)

	sshfsWrapper := filepath.Join(imageDir, "sshfs-wrapper")
	rootPrivKey := filepath.Join(imageDir, "/etc/ssh/ssh_host_rsa_key")
	userPrivKey := filepath.Join(imageDir, "user.key")

	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--force", "--sandbox", imageDir, "testdata/sshfs.def"),
		e2e.PostRun(func(t *testing.T) {
			if t.Failed() {
				return
			}
			content, err := os.ReadFile(rootPrivKey)
			if err != nil {
				t.Errorf("could not read ssh private key: %s", err)
				return
			}
			if err := os.WriteFile(userPrivKey, content, 0o600); err != nil {
				t.Errorf("could not write ssh user private key: %s", err)
				return
			}
			if err := os.Chown(userPrivKey, int(u.UID), int(u.GID)); err != nil {
				t.Errorf("could not change ssh user private key owner: %s", err)
				return
			}
		}),
		e2e.ExpectExit(0),
	)

	stdinReader, stdinWriter := io.Pipe()

	// we don't use an instance as it could conflict with
	// instance tests running in parallel
	go func() {
		c.env.RunApptainer(
			t,
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithStdin(stdinReader),
			e2e.WithCommand("run"),
			e2e.WithArgs("--writable", "--no-home", imageDir),
			e2e.PostRun(func(_ *testing.T) {
				stdinReader.Close()
				stdinWriter.Close()
			}),
			e2e.ExpectExit(0),
		)
	}()

	// terminate ssh server once done
	defer func() {
		stdinWriter.Write([]byte("bye"))
		stdinWriter.Close()
	}()

	// wait until ssh server is up and running
	retry := 0
	for {
		conn, err := net.Dial("tcp", "127.0.0.1:2022")
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(1 * time.Second)
		retry++
		if retry == 5 {
			t.Fatalf("ssh server unreachable after 5 seconds: %+v", err)
		}
	}

	basicTests := []struct {
		name    string
		spec    string
		key     string
		profile e2e.Profile
	}{
		{
			name:    "HostDaemonAsRoot",
			spec:    "host-daemon",
			key:     rootPrivKey,
			profile: e2e.RootProfile,
		},
		{
			name:    "HostAsRoot",
			spec:    "host",
			key:     rootPrivKey,
			profile: e2e.RootProfile,
		},
		{
			name:    "ContainerDaemonAsRoot",
			spec:    "container-daemon",
			key:     rootPrivKey,
			profile: e2e.RootProfile,
		},
		{
			name:    "ContainerAsRoot",
			spec:    "container",
			key:     rootPrivKey,
			profile: e2e.RootProfile,
		},
		{
			name:    "HostDaemonAsUser",
			spec:    "host-daemon",
			key:     userPrivKey,
			profile: e2e.UserProfile,
		},
		{
			name:    "HostAsUser",
			spec:    "host",
			key:     userPrivKey,
			profile: e2e.UserProfile,
		},
		{
			name:    "ContainerDaemonAsUser",
			spec:    "container-daemon",
			key:     userPrivKey,
			profile: e2e.UserProfile,
		},
		{
			name:    "ContainerAsUser",
			spec:    "container",
			key:     userPrivKey,
			profile: e2e.UserProfile,
		},
		{
			name:    "HostDaemonAsUserNamespace",
			spec:    "host-daemon",
			key:     userPrivKey,
			profile: e2e.UserNamespaceProfile,
		},
		{
			name:    "HostAsUserNamespace",
			spec:    "host",
			key:     userPrivKey,
			profile: e2e.UserNamespaceProfile,
		},
		{
			name:    "ContainerDaemonAsUserNamespace",
			spec:    "container-daemon",
			key:     userPrivKey,
			profile: e2e.UserNamespaceProfile,
		},
		{
			name:    "ContainerAsUserNamespace",
			spec:    "container",
			key:     userPrivKey,
			profile: e2e.UserNamespaceProfile,
		},
		{
			name:    "HostDaemonAsFakeroot",
			spec:    "host-daemon",
			key:     userPrivKey,
			profile: e2e.FakerootProfile,
		},
		{
			name:    "HostAsFakeroot",
			spec:    "host",
			key:     userPrivKey,
			profile: e2e.FakerootProfile,
		},
		{
			name:    "ContainerDaemonAsFakeroot",
			spec:    "container-daemon",
			key:     userPrivKey,
			profile: e2e.FakerootProfile,
		},
		{
			name:    "ContainerAsFakeroot",
			spec:    "container",
			key:     userPrivKey,
			profile: e2e.FakerootProfile,
		},
	}

	optionFmt := "%s:%s root@127.0.0.1:/ -p 2022 -F %s -o IdentityFile=%s -o StrictHostKeyChecking=no %s"
	sshConfig := filepath.Join(imageDir, "etc", "ssh", "ssh_config")

	for _, tt := range basicTests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs([]string{
				"--fusemount", fmt.Sprintf(optionFmt, tt.spec, sshfsWrapper, sshConfig, tt.key, "/mnt"),
				imageDir,
				"test", "-d", "/mnt/etc",
			}...),
			e2e.ExpectExit(0),
		)
	}
}

//nolint:maintidx
func (c actionTests) bindImage(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	require.Command(t, "mkfs.ext3")
	require.Command(t, "mksquashfs")
	require.Command(t, "dd")

	testdir, err := os.MkdirTemp(c.env.TestDir, "bind-image-")
	if err != nil {
		t.Fatal(err)
	}

	scratchDir := filepath.Join(testdir, "scratch")
	if err := os.MkdirAll(filepath.Join(scratchDir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}

	cleanup := func(t *testing.T) {
		if t.Failed() {
			t.Logf("Not removing directory %s for test %s", testdir, t.Name())
			return
		}
		err := os.RemoveAll(testdir)
		if err != nil {
			t.Logf("Error while removing directory %s for test %s: %#v", testdir, t.Name(), err)
		}
	}
	defer cleanup(t)

	sifSquashImage := filepath.Join(testdir, "data_squash.sif")
	sifExt3Image := filepath.Join(testdir, "data_ext3.sif")
	squashfsImage := filepath.Join(testdir, "squashfs.simg")
	ext3Img := filepath.Join(testdir, "ext3_fs.img")

	// create root directory for squashfs image
	squashDir, err := os.MkdirTemp(testdir, "root-squash-dir-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(squashDir, 0o755); err != nil {
		t.Fatal(err)
	}

	squashMarkerFile := "squash_marker"
	if err := fs.Touch(filepath.Join(squashDir, squashMarkerFile)); err != nil {
		t.Fatal(err)
	}

	// create the squashfs overlay image
	cmd := exec.Command("mksquashfs", squashDir, squashfsImage, "-noappend", "-all-root")
	if res := cmd.Run(t); res.Error != nil {
		t.Fatalf("Unexpected error while running command.\n%s", res)
	}

	// create the overlay ext3 image
	cmd = exec.Command("dd", "if=/dev/zero", "of="+ext3Img, "bs=1M", "count=64", "status=none")
	if res := cmd.Run(t); res.Error != nil {
		t.Fatalf("Unexpected error while running command.\n%s", res)
	}

	cmd = exec.Command("mkfs.ext3", "-q", "-F", ext3Img)
	if res := cmd.Run(t); res.Error != nil {
		t.Fatalf("Unexpected error while running command.\n%s", res)
	}

	// create new SIF images
	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("sif"),
		e2e.WithArgs([]string{"new", sifSquashImage}...),
		e2e.ExpectExit(0),
	)
	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("sif"),
		e2e.WithArgs([]string{"new", sifExt3Image}...),
		e2e.ExpectExit(0),
	)

	// arch partition doesn't matter for data partition so
	// take amd64 by default
	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("sif"),
		e2e.WithArgs([]string{
			"add",
			"--datatype", "4", "--partarch", "2",
			"--partfs", "1", "--parttype", "3",
			sifSquashImage, squashfsImage,
		}...),
		e2e.ExpectExit(0),
	)

	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("sif"),
		e2e.WithArgs([]string{
			"add",
			"--datatype", "4", "--partarch", "2",
			"--partfs", "2", "--parttype", "3",
			sifExt3Image, ext3Img,
		}...),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name    string
		profile e2e.Profile
		args    []string
		exit    int
	}{
		{
			name:    "NoBindOption",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", squashfsImage + ":/bind",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/bind", squashMarkerFile),
			},
			exit: 1,
		},
		{
			name:    "BadIDValue",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", squashfsImage + ":/bind:id=0",
				c.env.ImagePath,
				"true",
			},
			exit: 255,
		},
		{
			name:    "BadBindOption",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", squashfsImage + ":/bind:fake_option=fake",
				c.env.ImagePath,
				"true",
			},
			exit: 255,
		},
		{
			name:    "SandboxKO",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", squashDir + ":/bind:image-src=/",
				c.env.ImagePath,
				"true",
			},
			exit: 255,
		},
		{
			name:    "Squashfs",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", squashfsImage + ":/bind:image-src=/",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/bind", squashMarkerFile),
			},
			exit: 0,
		},
		{
			name:    "SquashfsDouble",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", squashfsImage + ":/bind1:image-src=/",
				"--bind", squashfsImage + ":/bind2:image-src=/",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/bind1", squashMarkerFile), "-a", "-f", filepath.Join("/bind2", squashMarkerFile),
			},
			exit: 0,
		},
		//Causes frequent crashes, not sure why.  See
		//  https://github.com/apptainer/apptainer/issues/1953
		// and possibly the fix to
		//  https://github.com/apptainer/apptainer/issues/2079
		// will affect it.
		//{
		//	name:    "SquashfsBadSource",
		//	profile: e2e.UserProfile,
		//	args:    []string{"--bind", squashfsImage + ":/bind:image-src=/ko", c.env.ImagePath, "true"},
		//	exit:    255,
		//},
		{
			name:    "SquashfsMixedBind",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", squashfsImage + ":/bind1:image-src=/",
				"--bind", squashDir + ":/bind2",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/bind1", squashMarkerFile), "-a", "-f", filepath.Join("/bind2", squashMarkerFile),
			},
			exit: 0,
		},
		{
			name:    "Ext3Write",
			profile: e2e.RootProfile,
			args: []string{
				"--bind", ext3Img + ":/bind:image-src=/",
				c.env.ImagePath,
				"touch", "/bind/ext3_marker",
			},
			exit: 0,
		},
		{
			name:    "Ext3WriteKO",
			profile: e2e.RootProfile,
			args: []string{
				"--bind", ext3Img + ":/bind:image-src=/,ro",
				c.env.ImagePath,
				"touch", "/bind/ext3_marker",
			},
			exit: 1,
		},
		{
			name:    "SifDataExt3Write",
			profile: e2e.RootProfile,
			args: []string{
				"--bind", sifExt3Image + ":/bind:image-src=/",
				c.env.ImagePath,
				"touch", "/bind/ext3_marker",
			},
			exit: 0,
		},
		{
			name:    "Ext3Read",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", ext3Img + ":/bind:image-src=/",
				c.env.ImagePath,
				"test", "-f", "/bind/ext3_marker",
			},
			exit: 0,
		},
		{
			name:    "Ext3Double",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", ext3Img + ":/bind1:image-src=/",
				"--bind", ext3Img + ":/bind2:image-src=/",
				c.env.ImagePath,
				"true",
			},
			exit: 255,
		},
		{
			name:    "SifDataSquash",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", sifSquashImage + ":/bind:image-src=/",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/bind", squashMarkerFile),
			},
			exit: 0,
		},
		{
			name:    "SifDataExt3Read",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", sifExt3Image + ":/bind:image-src=/",
				c.env.ImagePath,
				"test", "-f", "/bind/ext3_marker",
			},
			exit: 0,
		},
		{
			name:    "SifDataExt3AndSquash",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", sifExt3Image + ":/ext3:image-src=/",
				"--bind", sifSquashImage + ":/squash:image-src=/",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/squash", squashMarkerFile), "-a", "-f", "/ext3/ext3_marker",
			},
			exit: 0,
		},
		{
			name:    "SifDataExt3Double",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", sifExt3Image + ":/bind1:image-src=/",
				"--bind", sifExt3Image + ":/bind2:image-src=/",
				c.env.ImagePath,
				"true",
			},
			exit: 255,
		},
		{
			name:    "SifWithID",
			profile: e2e.UserProfile,
			args: []string{
				// rootfs ID is now '4'
				"--bind", c.env.ImagePath + ":/rootfs:id=4",
				c.env.ImagePath,
				"test", "-d", "/rootfs/etc",
			},
			exit: 0,
		},
		// check ordering between image and user bind
		{
			name:    "SquashfsBeforeScratch",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", sifSquashImage + ":/scratch/bin:image-src=/",
				"--bind", scratchDir + ":/scratch",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/scratch/bin", squashMarkerFile),
			},
			exit: 0,
		},
		{
			name:    "ScratchBeforeSquashfs",
			profile: e2e.UserProfile,
			args: []string{
				"--bind", scratchDir + ":/scratch",
				"--bind", sifSquashImage + ":/scratch/bin:image-src=/",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/scratch/bin", squashMarkerFile),
			},
			exit: 0,
		},
		// For the --mount variants we are really just verifying the CLI
		// acceptance of one or more image bind mount strings. Translation from
		// --mount strings to BindPath structs is checked in unit tests. The
		// functionality of image mounts of various kinds is already checked
		// above, with --bind flags. No need to duplicate all of these.
		{
			name:    "MountSifWithID",
			profile: e2e.UserProfile,
			args: []string{
				// rootfs ID is now '4'
				"--mount", "type=bind,source=" + c.env.ImagePath + ",destination=/rootfs,id=4",
				c.env.ImagePath,
				"test", "-d", "/rootfs/etc",
			},
			exit: 0,
		},
		{
			name:    "MountSifDataExt3AndSquash",
			profile: e2e.UserProfile,
			args: []string{
				"--mount", "type=bind,source=" + sifExt3Image + ",destination=/ext3,image-src=/",
				"--mount", "type=bind,source=" + sifSquashImage + ",destination=/squash,image-src=/",
				c.env.ImagePath,
				"test", "-f", filepath.Join("/squash", squashMarkerFile), "-a", "-f", "/ext3/ext3_marker",
			},
			exit: 0,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

// actionUmask tests that the within-container umask is correct in action flows
// Must be run in sequential section as it modifies host process umask.
func (c actionTests) actionUmask(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	u := e2e.UserProfile.HostUser(t)

	// Set umask for tests to 0000
	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)

	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.FakerootProfile),
		e2e.WithDir(u.Dir),
		e2e.WithCommand("exec"),
		e2e.WithArgs(c.env.ImagePath, "sh", "-c", "umask"),
		e2e.ExpectExit(
			0,
			e2e.ExpectOutput(e2e.ExactMatch, "0000"),
		),
	)

	c.env.RunApptainer(
		t,
		e2e.WithProfile(e2e.FakerootProfile),
		e2e.WithDir(u.Dir),
		e2e.WithCommand("exec"),
		e2e.WithArgs("--no-umask", c.env.ImagePath, "sh", "-c", "umask"),
		e2e.ExpectExit(
			0,
			e2e.ExpectOutput(e2e.ExactMatch, "0022"),
		),
	)
}

// actionUnsquash tests that the --unsquash option succeeds in conversion
func (c actionTests) actionUnsquash(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name    string
		profile e2e.Profile
	}{
		{
			name:    "user",
			profile: e2e.UserProfile,
		},
		{
			name:    "userns",
			profile: e2e.UserNamespaceProfile,
		},
		{
			name:    "fakeroot",
			profile: e2e.FakerootProfile,
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--unsquash", c.env.ImagePath, "true"),
			e2e.ExpectExit(
				0,
				e2e.ExpectError(e2e.ContainMatch, "Converting SIF"),
			),
		)
	}
}

func (c actionTests) actionNoMount(t *testing.T) {
	// TODO - this does not test --no-mount hostfs as that is a little tricky
	// We are in a mount namespace for e2e tests, so we can setup some mounts in there,
	// create a custom config with `mount hostfs = yes` set, and then look for presence
	// or absence of the mounts. I'd like to think about this a bit more though - work up
	// some nice helpers & cleanup for the actions we need.
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name string
		// Which mount directive to override (disable)
		noMount string
		// Output of `mount` command to ensure we should not find
		// e.g for `--no-mount home` "on /home" as mount command output is of the form:
		//   tmpfs on /home/dave type tmpfs (rw,seclabel,nosuid,nodev,relatime,size=16384k,uid=1000,gid=1000)
		noMatch string
		// Whether to run the test in default and/or contained modes
		// Needs to be specified as e.g. by default `/dev` mount is a full bind that will always include `/dev/pts`
		// ... but in --contained mode disabling devpts stops it being bound in.
		testDefault   bool
		testContained bool
		// To test --no-mount cwd we need to chdir for the execution
		cwd  string
		exit int
	}{
		{
			name:          "proc",
			noMount:       "proc",
			noMatch:       "on /proc",
			testDefault:   true,
			testContained: true,
			exit:          1, // mount fails with exit code 1 when there is no `/proc`
		},
		{
			name:          "sys",
			noMount:       "sys",
			noMatch:       "on /sys",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
		{
			name:          "dev",
			noMount:       "dev",
			noMatch:       "on /dev",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
		{
			name:          "devpts",
			noMount:       "devpts",
			noMatch:       "on /dev/pts",
			testDefault:   false,
			testContained: true,
			exit:          0,
		},
		{
			name:          "tmp",
			noMount:       "tmp",
			noMatch:       "on /tmp",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
		{
			name:          "home",
			noMount:       "home",
			noMatch:       "on /home",
			testDefault:   true,
			testContained: true,
			exit:          0,
			// run from / to avoid /home mount via CWD mount
			cwd: "/",
		},
		{
			// test by excluding both home and cwd without chdir
			name:          "home,cwd",
			noMount:       "home,cwd",
			noMatch:       "on /home",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
		{
			// /srv is an LSB directory we should be able to rely on for our CWD test
			name:        "cwd",
			noMount:     "cwd",
			noMatch:     "on /srv",
			testDefault: true,
			// CWD is never mounted with contain so --no-mount CWD doesn't have an effect,
			// but let's verify it isn't mounted anyway.
			testContained: true,
			cwd:           "/srv",
			exit:          0,
		},
		// /etc/hosts & /etc/localtime are default 'bind path' entries we should
		// be able to disable by abs path. Although other 'bind path' entries
		// are ignored under '--contain' these two are handled specially in
		// addBindsMount(), so make sure that `--no-mount` applies properly
		// under contain also.
		{
			name:          "/etc/hosts",
			noMount:       "/etc/hosts",
			noMatch:       "on /etc/hosts",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
		{
			name:          "/etc/localtime",
			noMount:       "/etc/localtime",
			noMatch:       "on /etc/localtime",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
		// bind-paths should disable all of the bind path mounts - including both defaults
		{
			name:          "binds-paths-hosts",
			noMount:       "bind-paths",
			noMatch:       "on /etc/hosts",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
		{
			name:          "bind-paths-localtime",
			noMount:       "bind-paths",
			noMatch:       "on /etc/localtime",
			testDefault:   true,
			testContained: true,
			exit:          0,
		},
	}

	for _, tt := range tests {

		if tt.testDefault {
			c.env.RunApptainer(
				t,
				e2e.WithDir(tt.cwd),
				e2e.AsSubtest(tt.name),
				e2e.WithProfile(e2e.UserProfile),
				e2e.WithCommand("exec"),
				e2e.WithArgs("--no-mount", tt.noMount, c.env.ImagePath, "mount"),
				e2e.ExpectExit(tt.exit,
					e2e.ExpectOutput(e2e.UnwantedContainMatch, tt.noMatch)),
			)
		}
		if tt.testContained {
			c.env.RunApptainer(
				t,
				e2e.WithDir(tt.cwd),
				e2e.AsSubtest(tt.name+"Contained"),
				e2e.WithProfile(e2e.UserProfile),
				e2e.WithCommand("exec"),
				e2e.WithArgs("--contain", "--no-mount", tt.noMount, c.env.ImagePath, "mount"),
				e2e.ExpectExit(tt.exit,
					e2e.ExpectOutput(e2e.UnwantedContainMatch, tt.noMatch)),
			)
		}
	}
}

// actionCompat checks that the --compat flag sets up the expected environment
// for improved oci/docker compatibility
// Must be run in sequential section as it modifies host process umask.
func (c actionTests) actionCompat(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	type test struct {
		name     string
		args     []string
		exitCode int
		expect   e2e.ApptainerCmdResultOp
	}

	tests := []test{
		{
			name:     "containall",
			args:     []string{"--compat", c.env.ImagePath, "sh", "-c", "ls -lah $HOME"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.ContainMatch, "total 0"),
		},
		{
			name:     "writable-tmpfs",
			args:     []string{"--compat", c.env.ImagePath, "sh", "-c", "touch /test"},
			exitCode: 0,
		},
		{
			name:     "no-init",
			args:     []string{"--compat", c.env.ImagePath, "sh", "-c", "ps"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.UnwantedContainMatch, "appinit"),
		},
		{
			name:     "no-umask",
			args:     []string{"--compat", c.env.ImagePath, "sh", "-c", "umask"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.ContainMatch, "0022"),
		},
	}

	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(
				tt.exitCode,
				tt.expect,
			),
		)
	}
}

// actionFakerootHome verifies that home dir is /root with --fakeroot
// (see: https://github.com/apptainer/apptainer/issues/618)
func (c actionTests) actionFakerootHome(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	type test struct {
		name     string
		args     []string
		exitCode int
		expect   e2e.ApptainerCmdResultOp
	}

	tests := []test{
		{
			name:     "fakeroot --with-suid",
			args:     []string{c.env.ImagePath, "sh", "-c", "echo $HOME"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.ExactMatch, "/root"),
		},
		{
			name:     "fakeroot --without-suid",
			args:     []string{"--userns", c.env.ImagePath, "sh", "-c", "echo $HOME"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.ExactMatch, "/root"),
		},
	}

	for _, tt := range tests {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.FakerootProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(
				tt.exitCode,
				tt.expect,
			),
		)
	}
}

// Make sure --workdir and --scratch work together nicely even when workdir is a
// relative path. Test needs to be run in non-parallel mode, because it changes
// the current working directory of the host.
func (c actionTests) relWorkdirScratch(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	testdir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "persistent-overlay-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			e2e.Privileged(cleanup)(t)
		}
	})

	const subdirName string = "mysubdir"
	if err := os.Mkdir(filepath.Join(testdir, subdirName), 0o777); err != nil {
		t.Fatalf("could not create subdirectory %q in %q: %s", subdirName, testdir, err)
	}

	// Change current working directory, with deferred undoing of change.
	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get current working directory: %s", err)
	}
	defer os.Chdir(prevCwd)
	if err = os.Chdir(testdir); err != nil {
		t.Fatalf("could not change cwd to %q: %s", testdir, err)
	}

	profiles := []e2e.Profile{e2e.UserProfile, e2e.RootProfile, e2e.FakerootProfile, e2e.UserNamespaceProfile}

	for _, p := range profiles {
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(p.String()),
			e2e.WithProfile(p),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--contain", "--workdir", "./"+subdirName, "--scratch", "/myscratch", c.env.ImagePath, "true"),
			e2e.ExpectExit(0),
		)
	}
}

// actionAuth tests run/exec/shell flows that involve authenticated pulls from
// OCI registries.
func (c actionTests) actionAuth(t *testing.T) {
	e2e.EnsureImage(t, c.env)
	profiles := []e2e.Profile{
		e2e.UserProfile,
		e2e.RootProfile,
	}

	for _, p := range profiles {
		t.Run(p.String(), func(t *testing.T) {
			t.Run("default", func(t *testing.T) {
				c.actionAuthTester(t, false, p)
			})
			t.Run("custom", func(t *testing.T) {
				c.actionAuthTester(t, true, p)
			})
		})
	}
}

func (c actionTests) actionAuthTester(t *testing.T, withCustomAuthFile bool, profile e2e.Profile) {
	tmpdir, tmpdirCleanup := e2e.MakeTempDir(t, c.env.TestDir, "action-auth", "")
	t.Cleanup(func() {
		if !t.Failed() {
			tmpdirCleanup(t)
		}
	})

	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get current working directory: %s", err)
	}
	defer os.Chdir(prevCwd)
	if err = os.Chdir(tmpdir); err != nil {
		t.Fatalf("could not change cwd to %q: %s", tmpdir, err)
	}

	localAuthFileName := ""
	if withCustomAuthFile {
		localAuthFileName = "./my_local_authfile"
	}

	authFileArgs := []string{}
	if withCustomAuthFile {
		authFileArgs = []string{"--authfile", localAuthFileName}
	}

	t.Cleanup(func() {
		e2e.PrivateRepoLogout(t, c.env, profile, localAuthFileName)
	})

	orasCustomPushTarget := fmt.Sprintf("oras://%s/authfile-pushtest-oras-busybox:latest", c.env.TestRegistryPrivPath)

	tests := []struct {
		name          string
		cmd           string
		args          []string
		whileLoggedIn bool
		expectExit    int
	}{
		{
			name:          "docker before auth",
			cmd:           "exec",
			args:          []string{"--disable-cache", "--no-https", c.env.TestRegistryPrivImage, "true"},
			whileLoggedIn: false,
			expectExit:    255,
		},
		{
			name:          "docker",
			cmd:           "exec",
			args:          []string{"--disable-cache", "--no-https", c.env.TestRegistryPrivImage, "true"},
			whileLoggedIn: true,
			expectExit:    0,
		},
		{
			name:          "noauth docker",
			cmd:           "exec",
			args:          []string{"--disable-cache", "--no-https", c.env.TestRegistryPrivImage, "true"},
			whileLoggedIn: false,
			expectExit:    255,
		},
		{
			name:          "oras push",
			cmd:           "push",
			args:          []string{c.env.ImagePath, orasCustomPushTarget},
			whileLoggedIn: true,
			expectExit:    0,
		},
		{
			name:          "noauth oras",
			cmd:           "exec",
			args:          []string{"--disable-cache", "--no-https", orasCustomPushTarget, "true"},
			whileLoggedIn: false,
			expectExit:    255,
		},
		{
			name:          "oras",
			cmd:           "exec",
			args:          []string{"--disable-cache", "--no-https", orasCustomPushTarget, "true"},
			whileLoggedIn: true,
			expectExit:    0,
		},
	}

	for _, tt := range tests {
		if tt.whileLoggedIn {
			e2e.PrivateRepoLogin(t, c.env, profile, localAuthFileName)
		} else {
			e2e.PrivateRepoLogout(t, c.env, profile, localAuthFileName)
		}
		c.env.RunApptainer(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(profile),
			e2e.WithCommand(tt.cmd),
			e2e.WithArgs(append(authFileArgs, tt.args...)...),
			e2e.ExpectExit(tt.expectExit),
		)
	}
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := actionTests{
		env: env,
	}

	np := testhelper.NoParallel

	return testhelper.Tests{
		"action URI":                   c.RunFromURI,             // action_URI
		"singularity link":             c.singularityLink,        // singularity symlink
		"exec":                         c.actionExec,             // apptainer exec
		"exec under multiple profiles": c.actionExecMultiProfile, // apptainer exec
		"run":                          c.actionRun,              // apptainer run
		"shell":                        c.actionShell,            // shell interaction
		"STDPIPE":                      c.STDPipe,                // stdin/stdout pipe
		"persistent overlay":           c.PersistentOverlay,
		"action basic profiles":        c.actionBasicProfiles,   // run basic action under different profiles
		"issue 4488":                   c.issue4488,             // https://github.com/apptainer/singularity/issues/4488
		"issue 4587":                   c.issue4587,             // https://github.com/apptainer/singularity/issues/4587
		"issue 4755":                   c.issue4755,             // https://github.com/apptainer/singularity/issues/4755
		"issue 4768":                   c.issue4768,             // https://github.com/apptainer/singularity/issues/4768
		"issue 4797":                   c.issue4797,             // https://github.com/apptainer/singularity/issues/4797
		"issue 4823":                   c.issue4823,             // https://github.com/apptainer/singularity/issues/4823
		"issue 4836":                   c.issue4836,             // https://github.com/apptainer/singularity/issues/4836
		"issue 5211":                   c.issue5211,             // https://github.com/apptainer/singularity/issues/5211
		"issue 5228":                   c.issue5228,             // https://github.com/apptainer/singularity/issues/5228
		"issue 5271":                   c.issue5271,             // https://github.com/apptainer/singularity/issues/5271
		"issue 5399":                   c.issue5399,             // https://github.com/apptainer/singularity/issues/5399
		"issue 5455":                   c.issue5455,             // https://github.com/apptainer/singularity/issues/5455
		"issue 5465":                   c.issue5465,             // https://github.com/apptainer/singularity/issues/5465
		"issue 5599":                   c.issue5599,             // https://github.com/apptainer/singularity/issues/5599
		"issue 5631":                   c.issue5631,             // https://github.com/apptainer/singularity/issues/5631
		"issue 5690":                   c.issue5690,             // https://github.com/apptainer/singularity/issues/5690
		"issue 6165":                   c.issue6165,             // https://github.com/apptainer/singularity/issues/6165
		"issue 619":                    c.issue619,              // https://github.com/apptainer/apptainer/issues/619
		"issue 1097":                   c.issue1097,             // https://github.com/apptainer/apptainer/issues/1097
		"issue 1848":                   c.issue1848,             // https://github.com/apptainer/apptainer/issues/1848
		"network":                      c.actionNetwork,         // test basic networking
		"netns-path":                   c.actionNetnsPath,       // test netns joining
		"binds":                        c.actionBinds,           // test various binds with --bind and --mount
		"layerType":                    c.actionLayerType,       // verify the various layer types
		"exit and signals":             c.exitSignals,           // test exit and signals propagation
		"fuse mount":                   c.fuseMount,             // test fusemount option
		"bind image":                   c.bindImage,             // test bind image with --bind and --mount
		"unsquash":                     c.actionUnsquash,        // test --unsquash
		"no-mount":                     c.actionNoMount,         // test --no-mount
		"compat":                       np(c.actionCompat),      // test --compat
		"umask":                        np(c.actionUmask),       // test umask propagation
		"invalidRemote":                np(c.invalidRemote),     // GHSA-5mv9-q7fq-9394
		"fakeroot home":                c.actionFakerootHome,    // test home dir in fakeroot
		"relWorkdirScratch":            np(c.relWorkdirScratch), // test relative --workdir with --scratch
		"issue 1868":                   c.issue1868,             // https://github.com/apptainer/apptainer/issues/1868
		"auth":                         np(c.actionAuth),        // tests action cmds w/authenticated pulls from OCI registries
	}
}
