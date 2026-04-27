// containerprobe exercises agent/container.CreateAndStart end-to-end against
// a local Docker daemon, then runs the GOAL-279 PoC success criteria
// against the resulting container so reviewers can see real evidence
// that the per-task isolation mechanism works on a real distro image.
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
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/container"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/jasper/options"
)

// toolchainBin is the directory inside the image where the buildhost-configuration
// toolchain installs its compilers. v4 is the current default for ubuntu2204.
const toolchainBin = "/opt/mongodbtoolchain/v4/bin"

func main() {
	image := flag.String("image", "evergreen-task-image/ubuntu2204:dev", "Docker image for the probe")
	memoryMB := flag.Int64("memory-mb", 0, "Memory limit in MB (0 for none)")
	cpus := flag.Int64("cpus", 0, "CPU limit in whole cores (0 for none)")
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

	// In the real agent flow, the agent reads container settings from
	// TaskConfig.Distro.ContainerIsolation (apimodels.ContainerIsolationSettings)
	// and uses TaskConfig.WorkDir + TaskConfig.Task.Id to populate a
	// container.Config; after CreateAndStart it sets TaskConfig.ContainerID.
	// `agent/internal` is a Go-internal package and can't be imported from
	// cmd/, so the harness drives container.Config directly while
	// constructing the same DistroView shape the agent would receive.
	taskID := fmt.Sprintf("probe-%d", time.Now().Unix())
	distro := &apimodels.DistroView{
		ContainerIsolation: &apimodels.ContainerIsolationSettings{
			Image:    *image,
			MemoryMB: *memoryMB,
			CPUs:     *cpus,
		},
	}

	fmt.Printf("→ CreateAndStart image=%s task=%s workdir=%s\n", *image, taskID, workDir)
	cnt, err := container.CreateAndStart(ctx, container.Config{
		Image:    distro.ContainerIsolation.Image,
		WorkDir:  workDir,
		TaskID:   taskID,
		MemoryMB: distro.ContainerIsolation.MemoryMB,
		CPUs:     distro.ContainerIsolation.CPUs,
	})
	if err != nil {
		log.Fatalf("CreateAndStart: %v", err)
	}
	fmt.Printf("✓ started container id=%s name=%s\n", cnt.ID[:12], cnt.Name)

	results := runChecks(ctx, cnt.ID, workDir)

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

func runChecks(ctx context.Context, containerID, workDir string) []checkResult {
	results := []checkResult{}
	// helpers ----------------------------------------------------------------

	// execIn runs argv inside the container with the toolchain on PATH and
	// /work as the working directory. Approach B from the PoC plan: the
	// agent's WrapWithContainer doesn't set --workdir or forward env, so
	// we build the docker-exec args directly when those matter.
	execIn := func(argv ...string) (string, error) {
		full := append([]string{
			"exec",
			"-w", container.WorkDirInContainer,
			"-e", "PATH=" + toolchainBin + ":/usr/local/bin:/usr/bin:/bin",
			containerID,
		}, argv...)
		out, err := exec.CommandContext(ctx, "docker", full...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	// execViaWrapper is the same idea via the agent's WrapWithContainer
	// helper — exercised at least once to prove the wrapper integrates
	// with the harness. WrapWithContainer doesn't add -w or -e, so this
	// path is suitable only for env-independent commands.
	execViaWrapper := func(argv ...string) (string, error) {
		opts := &options.Create{Args: argv}
		agentutil.WrapWithContainer(opts, containerID)
		out, err := exec.CommandContext(ctx, opts.Args[0], opts.Args[1:]...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	add := func(name string, pass bool, detail string) {
		results = append(results, checkResult{name, pass, detail})
		mark := "✗"
		if pass {
			mark = "✓"
		}
		fmt.Printf("  %s %s — %s\n", mark, name, detail)
	}

	// 1. Distro identity (VERSION_ID="22.04")
	out, err := execIn("sh", "-c", "grep ^VERSION_ID= /etc/os-release")
	add("distro identity", err == nil && strings.Contains(out, `"22.04"`), out)

	// 2. Hostname isolation — container hostname should be the container ID prefix,
	//    not the host's hostname. Use WrapWithContainer here as the wrapper-
	//    integration demonstration.
	hn, err := execViaWrapper("hostname")
	hostHN, _ := os.Hostname()
	add("hostname isolation", err == nil && hn != "" && hn != hostHN, fmt.Sprintf("container=%q host=%q", hn, hostHN))

	// 3. Toolchain availability — `which gcc` must resolve under /opt/mongodbtoolchain/.
	whichGcc, _ := execIn("which", "gcc")
	gccVer, gccErr := execIn("gcc", "--version")
	gccLine := strings.SplitN(gccVer, "\n", 2)[0]
	add("toolchain gcc on PATH", strings.HasPrefix(whichGcc, "/opt/mongodbtoolchain/") && gccErr == nil,
		fmt.Sprintf("which=%q version=%q", whichGcc, gccLine))

	// 4. Mounted workdir, host → container.
	hostMsg := "hello from host"
	if err := os.WriteFile(filepath.Join(workDir, "from-host.txt"), []byte(hostMsg+"\n"), 0o644); err != nil {
		add("host→container bind", false, "host write failed: "+err.Error())
	} else {
		got, err := execIn("cat", container.WorkDirInContainer+"/from-host.txt")
		add("host→container bind", err == nil && strings.TrimSpace(got) == hostMsg, got)
	}

	// 5. Mounted workdir, container → host.
	cMsg := "hello from container"
	_, err = execIn("sh", "-c", fmt.Sprintf("echo '%s' > %s/from-container.txt", cMsg, container.WorkDirInContainer))
	hostRead := ""
	if err == nil {
		b, rerr := os.ReadFile(filepath.Join(workDir, "from-container.txt"))
		if rerr == nil {
			hostRead = strings.TrimSpace(string(b))
		}
	}
	add("container→host bind", hostRead == cMsg, hostRead)

	// 6. UID/GID — harness runs container as root by default (no -u flag, no
	//    USER directive enforced). Plan's escape hatch: assert the default
	//    matches what buildhost-configuration would create on the host (root
	//    is the playbook's user= var for the container variant).
	uid, _ := execIn("id", "-u")
	gid, _ := execIn("id", "-g")
	add("uid/gid default user", uid == "0" && gid == "0", fmt.Sprintf("uid=%s gid=%s (image default; playbook sets user=root)", uid, gid))

	// 7. PID isolation — `ps -ef` should show only container processes,
	//    not host processes. The init shim from container.HostConfig.Init
	//    means PID 1 is `docker-init`, plus our `sleep infinity`.
	psOut, _ := execIn("ps", "-ef")
	psLines := strings.Count(psOut, "\n") + 1
	// Heuristic: container PID namespace should have a small process count
	// (single-digit), not hundreds like the host.
	add("pid isolation", psLines < 25, fmt.Sprintf("%d ps lines (container ns is small)", psLines))

	// 8. Network reachability — curl the public mongodb.com TLS endpoint.
	curlOut, curlErr := execIn("curl", "-sI", "-o", "/dev/null", "-w", "%{http_code}", "https://www.mongodb.com")
	add("network reachability", curlErr == nil && (curlOut == "200" || curlOut == "301" || curlOut == "302"), "HTTP "+curlOut)

	// 9-10. Cleanup checks deferred until after Destroy — see runPostDestroyChecks.
	return results
}

// runPostDestroyChecks verifies that Destroy actually cleaned up.
// Criteria 9 (no orphan container) and 10 (no orphan files in host workdir).
func runPostDestroyChecks(ctx context.Context, containerName, workDir string, destroyErr error) []checkResult {
	var out []checkResult

	// 9. No orphan container with the evergreen-task- name prefix remains.
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
	mark := "✗"
	if pass {
		mark = "✓"
	}
	fmt.Printf("  %s cleanup: no orphan container — %s\n", mark, detail)

	// 10. No orphan files in workdir beyond what the harness deliberately wrote.
	expected := map[string]bool{
		"from-host.txt":      true,
		"from-container.txt": true,
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
	detail = fmt.Sprintf("%d entries (expected: from-host.txt, from-container.txt)", len(entries))
	if len(unexpected) > 0 {
		detail += "; unexpected: " + strings.Join(unexpected, ",")
	}
	out = append(out, checkResult{"cleanup: no orphan files", pass, detail})
	mark = "✗"
	if pass {
		mark = "✓"
	}
	fmt.Printf("  %s cleanup: no orphan files — %s\n", mark, detail)

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
