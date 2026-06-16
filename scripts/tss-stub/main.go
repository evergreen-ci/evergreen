// tss-stub is a tiny stand-in for the test selection service used to verify
// the quarantine APIs end-to-end against a local Evergreen app server. It
// implements the TSS endpoints Evergreen calls, holds state in memory, and
// logs every request so you can see exactly what Evergreen sends.
//
// Usage:
//   go run ./scripts/tss-stub
//   go run ./scripts/tss-stub -port 9091
//
// Then point local Evergreen at it by setting the test_selection.url admin
// setting to http://localhost:9091 (or whatever port).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

type stateInfo struct {
	State         string  `json:"state"`
	OverrideState *string `json:"override_state,omitempty"`
}

type taskStateInfo struct {
	TaskName  string               `json:"task_name"`
	TestStats map[string]stateInfo `json:"test_stats"`
}

type store struct {
	mu                sync.Mutex
	variantQuarantine map[string]bool                       // key: project|bv
	taskQuarantine    map[string]bool                       // key: project|bv|task
	testQuarantine    map[string]map[string]bool            // key: project|bv|task -> test -> bool
	knownTasks        map[string]map[string]map[string]bool // project -> bv -> task -> seen
}

func newStore() *store {
	return &store{
		variantQuarantine: map[string]bool{},
		taskQuarantine:    map[string]bool{},
		testQuarantine:    map[string]map[string]bool{},
		knownTasks:        map[string]map[string]map[string]bool{},
	}
}

func (s *store) note(project, bv, task string) {
	if _, ok := s.knownTasks[project]; !ok {
		s.knownTasks[project] = map[string]map[string]bool{}
	}
	if _, ok := s.knownTasks[project][bv]; !ok {
		s.knownTasks[project][bv] = map[string]bool{}
	}
	if task != "" {
		s.knownTasks[project][bv][task] = true
	}
}

func (s *store) isTestQuarantined(project, bv, task, test string) bool {
	if tests, ok := s.testQuarantine[project+"|"+bv+"|"+task]; ok {
		if v, ok := tests[test]; ok {
			return v
		}
	}
	if v, ok := s.taskQuarantine[project+"|"+bv+"|"+task]; ok {
		return v
	}
	if v, ok := s.variantQuarantine[project+"|"+bv]; ok {
		return v
	}
	return false
}

func stateString(quarantined bool) string {
	if quarantined {
		return "manually_quarantined"
	}
	return "stable"
}

// trimSegments splits a URL path after the given prefix into trailing
// non-empty segments, tolerating TSS's trailing slashes.
func trimSegments(path, prefix string) []string {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil
	}
	return strings.Split(rest, "/")
}

func logRequest(r *http.Request, body []byte) {
	log.Printf("%s %s?%s  body=%s", r.Method, r.URL.Path, r.URL.RawQuery, string(body))
}

func main() {
	port := flag.Int("port", 9091, "port to listen on")
	flag.Parse()

	s := newStore()

	http.HandleFunc("/api/test_selection/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		logRequest(r, body)

		w.Header().Set("Content-Type", "application/json")
		s.mu.Lock()
		defer s.mu.Unlock()

		isQuarantined := r.URL.Query().Get("is_manually_quarantined") == "true"

		switch {
		case strings.Contains(r.URL.Path, "/transition_task/"):
			segs := trimSegments(r.URL.Path, "/api/test_selection/transition_task/")
			if len(segs) < 3 {
				http.Error(w, "bad path", http.StatusBadRequest)
				return
			}
			project, bv, task := segs[0], segs[1], segs[2]
			s.taskQuarantine[project+"|"+bv+"|"+task] = isQuarantined
			s.note(project, bv, task)
			_, _ = w.Write([]byte("{}"))

		case strings.Contains(r.URL.Path, "/transition_variant/"):
			segs := trimSegments(r.URL.Path, "/api/test_selection/transition_variant/")
			if len(segs) < 2 {
				http.Error(w, "bad path", http.StatusBadRequest)
				return
			}
			project, bv := segs[0], segs[1]
			s.variantQuarantine[project+"|"+bv] = isQuarantined
			s.note(project, bv, "")
			_, _ = w.Write([]byte("{}"))

		case strings.Contains(r.URL.Path, "/transition_tests/"):
			segs := trimSegments(r.URL.Path, "/api/test_selection/transition_tests/")
			if len(segs) < 3 {
				http.Error(w, "bad path", http.StatusBadRequest)
				return
			}
			project, bv, task := segs[0], segs[1], segs[2]
			var testNames []string
			if err := json.Unmarshal(body, &testNames); err != nil {
				http.Error(w, fmt.Sprintf("bad body: %v", err), http.StatusBadRequest)
				return
			}
			key := project + "|" + bv + "|" + task
			if _, ok := s.testQuarantine[key]; !ok {
				s.testQuarantine[key] = map[string]bool{}
			}
			for _, name := range testNames {
				s.testQuarantine[key][name] = isQuarantined
			}
			s.note(project, bv, task)
			_, _ = w.Write([]byte("{}"))

		case strings.Contains(r.URL.Path, "/get_variant_state/"):
			segs := trimSegments(r.URL.Path, "/api/test_selection/get_variant_state/")
			if len(segs) < 2 {
				http.Error(w, "bad path", http.StatusBadRequest)
				return
			}
			project, bv := segs[0], segs[1]
			out := map[string]taskStateInfo{}
			tasks := s.knownTasks[project][bv]
			if len(tasks) == 0 {
				// Synthesize a placeholder task so callers see a non-empty
				// shape that reflects the variant-level default.
				tasks = map[string]bool{"_default_task": true}
			}
			for task := range tasks {
				tests := s.testQuarantine[project+"|"+bv+"|"+task]
				if len(tests) == 0 {
					// No per-test overrides; synthesize one default test
					// reflecting task or variant default.
					tests = map[string]bool{"_default_test": s.isTestQuarantined(project, bv, task, "_default_test")}
				}
				testStats := map[string]stateInfo{}
				for testName := range tests {
					testStats[testName] = stateInfo{State: stateString(s.isTestQuarantined(project, bv, task, testName))}
				}
				out[task] = taskStateInfo{TaskName: task, TestStats: testStats}
			}
			_ = json.NewEncoder(w).Encode(out)

		case strings.Contains(r.URL.Path, "/get_tests_state/"):
			segs := trimSegments(r.URL.Path, "/api/test_selection/get_tests_state/")
			if len(segs) < 3 {
				http.Error(w, "bad path", http.StatusBadRequest)
				return
			}
			project, bv, task := segs[0], segs[1], segs[2]
			var testNames []string
			if err := json.Unmarshal(body, &testNames); err != nil {
				http.Error(w, fmt.Sprintf("bad body: %v", err), http.StatusBadRequest)
				return
			}
			out := map[string]stateInfo{}
			for _, name := range testNames {
				out[name] = stateInfo{State: stateString(s.isTestQuarantined(project, bv, task, name))}
			}
			s.note(project, bv, task)
			_ = json.NewEncoder(w).Encode(out)

		default:
			// Unknown TSS endpoint; return empty success so Evergreen doesn't
			// fail on calls we don't model yet.
			_, _ = w.Write([]byte("{}"))
		}
	})

	addr := fmt.Sprintf("localhost:%d", *port)
	log.Printf("tss-stub listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
