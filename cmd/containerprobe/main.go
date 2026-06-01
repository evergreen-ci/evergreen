// containerprobe exercises agent/container.CreateAndStart end-to-end against
// a local Docker daemon, then runs the GOAL-279 v2 PoC success criteria
// against the resulting container so reviewers can see real evidence
// that the per-task isolation mechanism works on a real distro image
// produced by the imagebuild filesystem-snapshot flow.
//
// Not part of any production build.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen/agent/container"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/jasper/options"
)

func main() {
	image := flag.String("image", "evergreen-task-image/ubuntu2204:dev", "Docker image for the probe")
	memoryMB := flag.Int64("memory-mb", 0, "Memory limit in MB (0 for none)")
	cpus := flag.Int64("cpus", 0, "CPU limit in whole cores (0 for none)")
	defaultOpt, _ := os.UserHomeDir()
	optPath := flag.String("opt-path", filepath.Join(defaultOpt, "poc-opt"), "Host directory bind-mounted read-only at /opt inside the container")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	home, _ := os.UserHomeDir()
	workDir, err := os.MkdirTemp(home, "containerprobe-")
	if err != nil {
		log.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(workDir)
	if resolved, err := filepath.EvalSymlinks(workDir); err == nil {
		workDir = resolved
	}

	// Auto-populate $optPath with a marker if it's empty so the harness works
	// out of the box. Real toolchains (Go, Python, etc.) can be dropped in here
	// later for a richer test — see the README.
	if err := ensureOptPopulated(*optPath); err != nil {
		log.Fatalf("populate opt-path: %v", err)
	}

	// Pre-flight criterion #3: with no /opt bind mount, ls /opt inside the
	// image should be empty — proves imagebuild's tarSystem exclude list
	// dropped /opt at image-build time.
	preflight := preflightImageExcludesOpt(ctx, *image)

	// In the real agent flow, the agent reads container settings from
	// TaskConfig.Distro.ContainerIsolation (apimodels.ContainerIsolationSettings)
	// and drives a container.Config from there. The harness constructs the
	// same DistroView shape and adds the v2-specific /opt bind mount.
	taskID := fmt.Sprintf("probe-%d", time.Now().Unix())
	distro := &apimodels.DistroView{
		ContainerIsolation: &apimodels.ContainerIsolationSettings{
			Image:    *image,
			MemoryMB: *memoryMB,
			CPUs:     *cpus,
		},
	}

	fmt.Printf("→ CreateAndStart image=%s task=%s workdir=%s opt=%s\n", *image, taskID, workDir, *optPath)
	cnt, err := container.CreateAndStart(ctx, container.Config{
		Image:    distro.ContainerIsolation.Image,
		WorkDir:  workDir,
		TaskID:   taskID,
		MemoryMB: distro.ContainerIsolation.MemoryMB,
		CPUs:     distro.ContainerIsolation.CPUs,
		ExtraMounts: []container.Mount{
			{Source: *optPath, Target: "/opt", ReadOnly: true},
		},
	})
	if err != nil {
		log.Fatalf("CreateAndStart: %v", err)
	}
	fmt.Printf("✓ started container id=%s name=%s\n", cnt.ID[:12], cnt.Name)

	results := []checkResult{preflight}
	results = append(results, runChecks(ctx, cnt.ID, workDir, *optPath)...)

	fmt.Println("→ Destroy")
	destroyErr := cnt.Destroy(context.Background())
	if destroyErr != nil {
		log.Printf("destroy: %v", destroyErr)
	} else {
		fmt.Println("✓ destroyed")
	}

	results = append(results, runPostDestroyChecks(ctx, cnt.Name, workDir, destroyErr)...)
	printSummary(results)

	pass := 0
	for _, r := range results {
		if r.pass {
			pass++
		}
	}
	if pass != len(results) {
		os.Exit(1)
	}
}

type checkResult struct {
	name   string
	pass   bool
	detail string
}

// getSysUID extracts the owning UID from a unix stat result. The PoC only
// runs on macOS/Linux, so a direct *syscall.Stat_t assertion is sufficient.
func getSysUID(fi os.FileInfo) (int, bool) {
	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(s.Uid), true
}

func mark(pass bool) string {
	if pass {
		return "✓"
	}
	return "✗"
}

// ensureOptPopulated drops a small marker into optPath if it is missing or
// empty. Lets the harness run out of the box without requiring the operator
// to install a real toolchain first.
func ensureOptPopulated(optPath string) error {
	if err := os.MkdirAll(optPath, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(optPath)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	markerDir := filepath.Join(optPath, "marker", "bin")
	if err := os.MkdirAll(markerDir, 0o755); err != nil {
		return err
	}
	script := "#!/bin/sh\necho 'PoC v2 toolchain marker'\n"
	return os.WriteFile(filepath.Join(markerDir, "version"), []byte(script), 0o755)
}

// preflightImageExcludesOpt runs `docker run --rm <image> ls /opt` and asserts
// the output is empty (the imagebuild exclude list dropped /opt during snapshot).
func preflightImageExcludesOpt(ctx context.Context, image string) checkResult {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", image, "sh", "-c", "ls /opt 2>/dev/null | head")
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	pass := err == nil && output == ""
	r := checkResult{
		name:   "image excludes /opt",
		pass:   pass,
		detail: fmt.Sprintf("ls /opt (no bind mount) → %q", output),
	}
	fmt.Printf("  %s %s — %s\n", mark(pass), r.name, r.detail)
	return r
}

func runChecks(ctx context.Context, containerID, workDir, optPath string) []checkResult {
	results := []checkResult{}
	hostUID := strconv.Itoa(os.Getuid())

	// execIn runs argv inside the container with workDir as cwd (same-path semantics).
	execIn := func(argv ...string) (string, error) {
		full := append([]string{"exec", "-w", workDir, containerID}, argv...)
		out, err := exec.CommandContext(ctx, "docker", full...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	// execAs is the same but with -u <uid>:<gid> so the kernel sees the host's user.
	execAs := func(uid string, argv ...string) (string, error) {
		full := append([]string{"exec", "-u", uid, "-w", workDir, containerID}, argv...)
		out, err := exec.CommandContext(ctx, "docker", full...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	// One demonstrative use of agent/util.WrapWithContainer to prove the
	// wrapper integrates with downstream callers.
	execViaWrapper := func(argv ...string) (string, error) {
		opts := &options.Create{Args: argv}
		if err := agentutil.WrapWithContainer(opts, containerID, workDir, ""); err != nil {
			return "", err
		}
		out, err := exec.CommandContext(ctx, opts.Args[0], opts.Args[1:]...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	add := func(name string, pass bool, detail string) {
		results = append(results, checkResult{name, pass, detail})
		fmt.Printf("  %s %s — %s\n", mark(pass), name, detail)
	}

	// 1. Distro identity.
	out, err := execIn("sh", "-c", "grep ^VERSION_ID= /etc/os-release")
	add("distro identity", err == nil && strings.Contains(out, `"22.04"`), out)

	// 2. Hostname isolation. Use WrapWithContainer here as the wrapper-
	//    integration demonstration.
	hn, err := execViaWrapper("hostname")
	hostHN, _ := os.Hostname()
	add("hostname isolation", err == nil && hn != "" && hn != hostHN, fmt.Sprintf("container=%q host=%q", hn, hostHN))

	// 4. Toolchain visible via /opt bind mount. ls /opt inside the container
	//    should match ls $optPath on the host.
	insideLS, _ := execIn("sh", "-c", "ls /opt | sort")
	hostEntries := []string{}
	if hostList, err := os.ReadDir(optPath); err == nil {
		for _, e := range hostList {
			hostEntries = append(hostEntries, e.Name())
		}
	}
	sort.Strings(hostEntries)
	expectedLS := strings.Join(hostEntries, "\n")
	add("toolchain via /opt bind", insideLS == expectedLS && insideLS != "",
		fmt.Sprintf("inside=%q host=%q", insideLS, expectedLS))

	// 5. /opt is read-only. touch /opt/foo should fail.
	_, touchErr := execIn("sh", "-c", "touch /opt/foo 2>&1")
	add("/opt is read-only", touchErr != nil, fmt.Sprintf("touch /opt/foo err=%v", touchErr))

	// 6. Workdir bind, host → container.
	hostMsg := "hello from host"
	if err := os.WriteFile(filepath.Join(workDir, "from-host.txt"), []byte(hostMsg+"\n"), 0o644); err != nil {
		add("host→container bind", false, "host write failed: "+err.Error())
	} else {
		got, err := execIn("cat", workDir+"/from-host.txt")
		add("host→container bind", err == nil && strings.TrimSpace(got) == hostMsg, got)
	}

	// 7. Workdir bind, container → host.
	cMsg := "hello from container"
	_, err = execIn("sh", "-c", fmt.Sprintf("echo '%s' > '%s/from-container.txt'", cMsg, workDir))
	hostRead := ""
	if err == nil {
		b, rerr := os.ReadFile(filepath.Join(workDir, "from-container.txt"))
		if rerr == nil {
			hostRead = strings.TrimSpace(string(b))
		}
	}
	add("container→host bind", hostRead == cMsg, hostRead)

	// 8. UID/GID consistency. With `docker exec -u <hostUID>` the kernel
	//    sees the host's UID inside the container, and a file written
	//    under the bind-mounted workdir is owned by host UID on the host.
	uidIn, _ := execAs(hostUID, "id", "-u")
	_, _ = execAs(hostUID, "sh", "-c", "touch '"+workDir+"/uid-probe.txt'")
	stat, statErr := os.Stat(filepath.Join(workDir, "uid-probe.txt"))
	fileUID := -1
	if statErr == nil {
		if sys, ok := getSysUID(stat); ok {
			fileUID = sys
		}
	}
	uidOK := uidIn == hostUID && fileUID == os.Getuid()
	add("uid/gid consistency", uidOK, fmt.Sprintf("hostUID=%s containerUID=%s fileOwnerUID=%d", hostUID, uidIn, fileUID))

	// 9. PID isolation — `ps -ef` should show only container processes.
	psOut, _ := execIn("ps", "-ef")
	psLines := strings.Count(psOut, "\n") + 1
	add("pid isolation", psLines < 25, fmt.Sprintf("%d ps lines (container ns is small)", psLines))

	return results
}

// runPostDestroyChecks verifies that Destroy actually cleaned up.
func runPostDestroyChecks(ctx context.Context, containerName, workDir string, destroyErr error) []checkResult {
	var out []checkResult

	// 10. No orphan container with the evergreen-task- name prefix remains.
	psOut, psErr := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--filter", "name="+containerName,
		"--format", "{{.Names}}").CombinedOutput()
	psStr := strings.TrimSpace(string(psOut))
	pass := destroyErr == nil && psErr == nil && psStr == ""
	detail := fmt.Sprintf("docker ps filter name=%s → %q", containerName, psStr)
	if destroyErr != nil {
		detail = "destroy errored: " + destroyErr.Error()
	}
	out = append(out, checkResult{"cleanup: no orphan container", pass, detail})
	fmt.Printf("  %s cleanup: no orphan container — %s\n", mark(pass), detail)

	// 11. No orphan files in workdir beyond what the harness deliberately wrote.
	expected := map[string]bool{
		"from-host.txt":      true,
		"from-container.txt": true,
		"uid-probe.txt":      true,
	}
	entries, err := os.ReadDir(workDir)
	unexpected := []string{}
	if err == nil {
		for _, e := range entries {
			if !expected[e.Name()] {
				unexpected = append(unexpected, e.Name())
			}
		}
	}
	pass = err == nil && len(unexpected) == 0
	detail = fmt.Sprintf("%d entries in workdir", len(entries))
	if len(unexpected) > 0 {
		detail += "; unexpected: " + strings.Join(unexpected, ",")
	}
	out = append(out, checkResult{"cleanup: no orphan files", pass, detail})
	fmt.Printf("  %s cleanup: no orphan files — %s\n", mark(pass), detail)

	return out
}

func printSummary(results []checkResult) {
	pass := 0
	for _, r := range results {
		if r.pass {
			pass++
		}
	}
	fmt.Printf("\n=== %d/%d criteria passed ===\n", pass, len(results))
	if pass != len(results) {
		fmt.Println("FAILED criteria:")
		for _, r := range results {
			if !r.pass {
				fmt.Printf("  - %s: %s\n", r.name, r.detail)
			}
		}
	}
}
