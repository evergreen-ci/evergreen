// Command feature-tracker analyzes which Evergreen features each project uses.
//
// It consumes a directory of project config files produced by
// `evergreen admin all-configs`, which downloads each project's most recent
// merged parser project as YAML (one `<identifier>.yml` per project, with
// `include`d files already resolved). For every file it re-parses the config
// into a model.Project using Evergreen's own parser and runs a registry of
// feature detectors, then emits a CSV, an HTML report, and a terminal summary.
//
// Usage:
//
//	evergreen admin all-configs --directory configs   # dump enabled projects' configs into ./configs
//	go run ./cmd/feature-tracker --dir configs --out feature-report
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
)

// Detector reports whether a project uses a feature and how heavily. The count
// semantics depend on the detector (e.g. number of display tasks, number of
// task groups, number of times a command is invoked); a count of zero means
// the feature is unused.
// Detect receives both the translated project and the parser project it came
// from. Most features are visible on the translated Project; some (e.g.
// matrices) only exist on the ParserProject because translation expands them
// away. pp may be nil if parsing failed.
type Detector struct {
	Name        string
	Description string
	Detect      func(p *model.Project, pp *model.ParserProject) int
}

// detectors returns the ordered registry of feature detectors. The first three
// are the features under active maintenance-cost review (display tasks, task
// groups, generate.tasks); the rest are a generalized set demonstrating that
// any structural feature or command usage can be tracked the same way.
func detectors() []Detector {
	return []Detector{
		{
			Name:        "display_tasks",
			Description: "Display tasks grouping execution tasks in build variants",
			Detect: func(p *model.Project, _ *model.ParserProject) int {
				count := 0
				for _, bv := range p.BuildVariants {
					count += len(bv.DisplayTasks)
				}
				return count
			},
		},
		{
			Name:        "task_groups",
			Description: "Task groups with shared setup/teardown",
			Detect: func(p *model.Project, _ *model.ParserProject) int {
				return len(p.TaskGroups)
			},
		},
		commandDetector("generate_tasks", "generate.tasks command (runtime task generation)", "generate.tasks"),

		// Generalized set: structural features.
		{
			Name:        "modules",
			Description: "Additional source modules pulled into the build",
			Detect: func(p *model.Project, _ *model.ParserProject) int {
				return len(p.Modules)
			},
		},
		{
			Name:        "matrices",
			Description: "Matrix build variant definitions (counted on the parser project, before expansion)",
			Detect: func(_ *model.Project, pp *model.ParserProject) int {
				if pp == nil {
					return 0
				}
				count := 0
				for _, bv := range pp.BuildVariants {
					if bv.Matrix != nil {
						count++
					}
				}
				return count
			},
		},

		// Generalized set: command usage. Each counts total invocations across
		// all tasks (including commands reached indirectly through functions).
		commandDetector("host_create", "host.create (dynamic host provisioning)", "host.create"),
		commandDetector("cache_save", "cache.save (new caching command)", "cache.save"),
		commandDetector("cache_restore", "cache.restore (new caching command)", "cache.restore"),
		commandDetector("s3_put", "s3.put", "s3.put"),
		commandDetector("s3_get", "s3.get", "s3.get"),
		commandDetector("subprocess_exec", "subprocess.exec", "subprocess.exec"),
		commandDetector("shell_exec", "shell.exec", "shell.exec"),
		commandDetector("manifest_load", "manifest.load", "manifest.load"),
		commandDetector("attach_results", "attach.results", "attach.results"),
		commandDetector("attach_xunit_results", "attach.xunit_results", "attach.xunit_results"),
		commandDetector("perf_send", "perf.send", "perf.send"),
		commandDetector("ec2_assume_role", "ec2.assume_role", "ec2.assume_role"),
	}
}

// commandDetector builds a Detector that counts total invocations of a command
// across all of a project's tasks. TasksThatCallCommand resolves commands
// called directly and those reached through functions.
func commandDetector(name, description, command string) Detector {
	return Detector{
		Name:        name,
		Description: description,
		Detect: func(p *model.Project, _ *model.ParserProject) int {
			total := 0
			for _, n := range p.TasksThatCallCommand(command) {
				total += n
			}
			return total
		},
	}
}

// result holds the per-project detector counts and any parse error.
type result struct {
	Project string
	Counts  map[string]int
	Err     string
}

func main() {
	dir := flag.String("dir", ".", "directory containing <project>.yml configs from `evergreen admin all-configs`")
	out := flag.String("out", "feature-report", "output file prefix; writes <prefix>.csv and <prefix>.html")
	flag.Parse()

	dets := detectors()

	files, err := filepath.Glob(filepath.Join(*dir, "*.yml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "globbing config files: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no *.yml files found in %q; run `evergreen admin all-configs` first\n", *dir)
		os.Exit(1)
	}

	results := make([]result, 0, len(files))
	for _, f := range files {
		results = append(results, analyzeFile(f, dets))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Project < results[j].Project })

	if err := writeCSV(*out+".csv", dets, results); err != nil {
		fmt.Fprintf(os.Stderr, "writing CSV: %v\n", err)
		os.Exit(1)
	}
	if err := writeHTML(*out+".html", dets, results); err != nil {
		fmt.Fprintf(os.Stderr, "writing HTML: %v\n", err)
		os.Exit(1)
	}
	printSummary(dets, results, *out)
}

// analyzeFile parses one config file and runs every detector against it. A
// parse failure is recorded on the result rather than aborting the run, since
// one malformed config should not block analysis of the rest.
func analyzeFile(path string, dets []Detector) result {
	projectID := strings.TrimSuffix(filepath.Base(path), ".yml")
	res := result{Project: projectID, Counts: map[string]int{}}

	data, err := os.ReadFile(path)
	if err != nil {
		res.Err = fmt.Sprintf("reading file: %v", err)
		return res
	}

	// The dumped config is a merged parser project whose Include field has been
	// cleared, so LoadProjectInto needs no network or DB access; nil opts is safe.
	var project model.Project
	pp, err := model.LoadProjectInto(context.Background(), data, nil, projectID, &project)
	if err != nil {
		res.Err = fmt.Sprintf("parsing project: %v", err)
		// LoadProjectInto fills the project even on error, so continue and detect.
	}

	for _, d := range dets {
		res.Counts[d.Name] = d.Detect(&project, pp)
	}
	return res
}

func writeCSV(path string, dets []Detector, results []result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"project"}
	for _, d := range dets {
		header = append(header, d.Name)
	}
	header = append(header, "parse_error")
	if err := w.Write(header); err != nil {
		return err
	}

	for _, r := range results {
		row := []string{r.Project}
		for _, d := range dets {
			row = append(row, strconv.Itoa(r.Counts[d.Name]))
		}
		row = append(row, r.Err)
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

// adoption summarizes how many projects use a feature.
type adoption struct {
	Name        string
	Description string
	Using       int
	Total       int
	Percent     string
}

func computeAdoption(dets []Detector, results []result) []adoption {
	total := len(results)
	out := make([]adoption, 0, len(dets))
	for _, d := range dets {
		using := 0
		for _, r := range results {
			if r.Counts[d.Name] > 0 {
				using++
			}
		}
		pct := 0.0
		if total > 0 {
			pct = 100 * float64(using) / float64(total)
		}
		out = append(out, adoption{
			Name:        d.Name,
			Description: d.Description,
			Using:       using,
			Total:       total,
			Percent:     fmt.Sprintf("%.1f%%", pct),
		})
	}
	return out
}

func printSummary(dets []Detector, results []result, outPrefix string) {
	parseErrors := 0
	for _, r := range results {
		if r.Err != "" {
			parseErrors++
		}
	}

	fmt.Printf("Analyzed %d projects (%d had parse errors)\n\n", len(results), parseErrors)

	adopt := computeAdoption(dets, results)
	// Sort the printed summary by adoption descending for a quick read.
	sorted := make([]adoption, len(adopt))
	copy(sorted, adopt)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Using > sorted[j].Using })

	width := 0
	for _, a := range sorted {
		if len(a.Name) > width {
			width = len(a.Name)
		}
	}
	fmt.Println("Feature adoption (projects using / total):")
	for _, a := range sorted {
		fmt.Printf("  %-*s  %4d / %-4d  %6s\n", width, a.Name, a.Using, a.Total, a.Percent)
	}
	fmt.Printf("\nWrote %s.csv and %s.html\n", outPrefix, outPrefix)
}

func writeHTML(path string, dets []Detector, results []result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	adopt := computeAdoption(dets, results)
	// Default the rendered order to most-used first; the table is re-sortable client-side.
	sort.SliceStable(adopt, func(i, j int) bool { return adopt[i].Using > adopt[j].Using })
	parseErrors := 0
	for _, r := range results {
		if r.Err != "" {
			parseErrors++
		}
	}

	// rows holds the per-project matrix in template-friendly form.
	type cell struct {
		Count int
		Used  bool
	}
	type row struct {
		Project string
		Cells   []cell
		Err     string
	}
	rows := make([]row, 0, len(results))
	for _, r := range results {
		cells := make([]cell, 0, len(dets))
		for _, d := range dets {
			c := r.Counts[d.Name]
			cells = append(cells, cell{Count: c, Used: c > 0})
		}
		rows = append(rows, row{Project: r.Project, Cells: cells, Err: r.Err})
	}

	data := struct {
		Detectors   []Detector
		Adoption    []adoption
		Rows        []row
		Total       int
		ParseErrors int
	}{
		Detectors:   dets,
		Adoption:    adopt,
		Rows:        rows,
		Total:       len(results),
		ParseErrors: parseErrors,
	}

	return htmlTemplate.Execute(f, data)
}

var htmlTemplate = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Evergreen Project Feature Usage</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         background: #fafafa; color: #1a1a1a; margin: 2rem; }
  h1 { font-size: 1.5rem; }
  .meta { color: #555; margin-bottom: 1.5rem; }
  table { border-collapse: collapse; background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.08);
          margin-bottom: 2.5rem; }
  th, td { border: 1px solid #e2e2e2; padding: 0.4rem 0.7rem; text-align: left; font-size: 0.85rem; }
  th { background: #f0f0f0; position: sticky; top: 0; }
  th.sortable { cursor: pointer; }
  th.sortable::after { content: " ↕"; color: #aaa; font-weight: normal; }
  th.sortable[data-dir="asc"]::after { content: " ↑"; color: #333; }
  th.sortable[data-dir="desc"]::after { content: " ↓"; color: #333; }
  th.feature { writing-mode: vertical-rl; transform: rotate(180deg); white-space: nowrap; height: 9rem; }
  td.num { text-align: right; font-variant-numeric: tabular-nums; }
  tr:nth-child(even) td { background: #fbfbfb; }
  .used { background: #d8efd8 !important; font-weight: 600; }
  .err { color: #b00020; font-size: 0.75rem; }
  caption { text-align: left; font-weight: 600; font-size: 1.05rem; padding-bottom: 0.5rem; }
</style>
</head>
<body>
<h1>Evergreen Project Feature Usage</h1>
<p class="meta">{{.Total}} projects analyzed{{if .ParseErrors}}, {{.ParseErrors}} with parse errors{{end}}.</p>

<table id="adoption" data-sort-col="2" data-sort-dir="desc">
  <caption>Feature adoption</caption>
  <thead><tr>
    <th class="sortable" onclick="sortTable('adoption', 0, false)">Feature</th>
    <th>Description</th>
    <th class="sortable" data-dir="desc" onclick="sortTable('adoption', 2, true)">Projects using</th>
    <th>%</th>
  </tr></thead>
  <tbody>
  {{range .Adoption}}
    <tr>
      <td><code>{{.Name}}</code></td>
      <td>{{.Description}}</td>
      <td class="num">{{.Using}} / {{.Total}}</td>
      <td class="num">{{.Percent}}</td>
    </tr>
  {{end}}
  </tbody>
</table>

<table id="matrix">
  <caption>Per-project feature matrix (counts; click a header to sort)</caption>
  <thead>
    <tr>
      <th class="sortable" onclick="sortTable('matrix', 0, false)">Project</th>
      {{range .Detectors}}<th class="feature" title="{{.Description}}">{{.Name}}</th>{{end}}
      <th>Parse error</th>
    </tr>
  </thead>
  <tbody>
  {{range .Rows}}
    <tr>
      <td>{{.Project}}</td>
      {{range .Cells}}<td class="num{{if .Used}} used{{end}}">{{.Count}}</td>{{end}}
      <td class="err">{{.Err}}</td>
    </tr>
  {{end}}
  </tbody>
</table>

<script>
// Minimal client-side column sort, shared by both tables.
function sortTable(tableId, col, numeric) {
  const table = document.getElementById(tableId);
  const tbody = table.tBodies[0];
  const rows = Array.from(tbody.rows);
  const asc = table.getAttribute("data-sort-col") != col || table.getAttribute("data-sort-dir") != "asc";
  rows.sort((a, b) => {
    let x = a.cells[col].innerText, y = b.cells[col].innerText;
    if (numeric) { x = parseFloat(x) || 0; y = parseFloat(y) || 0; return asc ? x - y : y - x; }
    return asc ? x.localeCompare(y) : y.localeCompare(x);
  });
  rows.forEach(r => tbody.appendChild(r));
  table.setAttribute("data-sort-col", col);
  table.setAttribute("data-sort-dir", asc ? "asc" : "desc");
  // Move the active sort caret to the clicked column.
  const headers = table.tHead.rows[0].cells;
  for (const h of headers) h.removeAttribute("data-dir");
  headers[col].setAttribute("data-dir", asc ? "asc" : "desc");
}
</script>
</body>
</html>
`))
