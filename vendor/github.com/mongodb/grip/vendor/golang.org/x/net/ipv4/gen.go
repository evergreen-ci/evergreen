// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

//go:generate go run gen.go

// This program generates system adaptation constants and types,
// internet protocol constants and tables by reading template files
// and IANA protocol registries.
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"go/format"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func main() {
	if err := genzsys(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := geniana(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func genzsys() error {
	defs := "defs_" + runtime.GOOS + ".go"
	f, err := os.Open(defs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	f.Close()
	cmd := exec.Command("go", "tool", "cgo", "-godefs", defs)
	b, err := cmd.Output()
	if err != nil {
		return err
	}
	// The ipv4 package still supports go1.2, and so we need to
	// take care of additional platforms in go1.3 and above for
	// working with go1.2.
	switch {
	case runtime.GOOS == "dragonfly" || runtime.GOOS == "solaris":
		b = bytes.Replace(b, []byte("package ipv4\n"), []byte("// +build "+runtime.GOOS+"\n\npackage ipv4\n"), 1)
	case runtime.GOOS == "linux" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "mips64" || runtime.GOARCH == "mips64le" || runtime.GOARCH == "ppc" || runtime.GOARCH == "ppc64" || runtime.GOARCH == "ppc64le" || runtime.GOARCH == "s390x"):
		b = bytes.Replace(b, []byte("package ipv4\n"), []byte("// +build "+runtime.GOOS+","+runtime.GOARCH+"\n\npackage ipv4\n"), 1)
	}
	b, err = format.Source(b)
	if err != nil {
		return err
	}
	zsys := "zsys_" + runtime.GOOS + ".go"
	switch runtime.GOOS {
	case "freebsd", "linux":
		zsys = "zsys_" + runtime.GOOS + "_" + runtime.GOARCH + ".go"
	}
	if err := ioutil.WriteFile(zsys, b, 0644); err != nil {
		return err
	}
	return nil
}

var registries = []struct {
	url   string
	parse func(io.Writer, io.Reader) error
}{
	{
		"http://www.iana.org/assignments/icmp-parameters/icmp-parameters.xml",
		parseICMPv4Parameters,
	},
}

func geniana() error {
	var bb bytes.Buffer
	fmt.Fprintf(&bb, "// go generate gen.go\n")
	fmt.Fprintf(&bb, "// GENERATED BY THE COMMAND ABOVE; DO NOT EDIT\n\n")
	fmt.Fprintf(&bb, "package ipv4\n\n")
	for _, r := range registries {
		resp, err := http.Get(r.url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("got HTTP status code %v for %v\n", resp.StatusCode, r.url)
		}
		if err := r.parse(&bb, resp.Body); err != nil {
			return err
		}
		fmt.Fprintf(&bb, "\n")
	}
	b, err := format.Source(bb.Bytes())
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile("iana.go", b, 0644); err != nil {
		return err
	}
	return nil
}

func parseICMPv4Parameters(w io.Writer, r io.Reader) error {
	dec := xml.NewDecoder(r)
	var icp icmpv4Parameters
	if err := dec.Decode(&icp); err != nil {
		return err
	}
	prs := icp.escape()
	fmt.Fprintf(w, "// %s, Updated: %s\n", icp.Title, icp.Updated)
	fmt.Fprintf(w, "const (\n")
	for _, pr := range prs {
		if pr.Descr == "" {
			continue
		}
		fmt.Fprintf(w, "ICMPType%s ICMPType = %d", pr.Descr, pr.Value)
		fmt.Fprintf(w, "// %s\n", pr.OrigDescr)
	}
	fmt.Fprintf(w, ")\n\n")
	fmt.Fprintf(w, "// %s, Updated: %s\n", icp.Title, icp.Updated)
	fmt.Fprintf(w, "var icmpTypes = map[ICMPType]string{\n")
	for _, pr := range prs {
		if pr.Descr == "" {
			continue
		}
		fmt.Fprintf(w, "%d: %q,\n", pr.Value, strings.ToLower(pr.OrigDescr))
	}
	fmt.Fprintf(w, "}\n")
	return nil
}

type icmpv4Parameters struct {
	XMLName    xml.Name `xml:"registry"`
	Title      string   `xml:"title"`
	Updated    string   `xml:"updated"`
	Registries []struct {
		Title   string `xml:"title"`
		Records []struct {
			Value string `xml:"value"`
			Descr string `xml:"description"`
		} `xml:"record"`
	} `xml:"registry"`
}

type canonICMPv4ParamRecord struct {
	OrigDescr string
	Descr     string
	Value     int
}

func (icp *icmpv4Parameters) escape() []canonICMPv4ParamRecord {
	id := -1
	for i, r := range icp.Registries {
		if strings.Contains(r.Title, "Type") || strings.Contains(r.Title, "type") {
			id = i
			break
		}
	}
	if id < 0 {
		return nil
	}
	prs := make([]canonICMPv4ParamRecord, len(icp.Registries[id].Records))
	sr := strings.NewReplacer(
		"Messages", "",
		"Message", "",
		"ICMP", "",
		"+", "P",
		"-", "",
		"/", "",
		".", "",
		" ", "",
	)
	for i, pr := range icp.Registries[id].Records {
		if strings.Contains(pr.Descr, "Reserved") ||
			strings.Contains(pr.Descr, "Unassigned") ||
			strings.Contains(pr.Descr, "Deprecated") ||
			strings.Contains(pr.Descr, "Experiment") ||
			strings.Contains(pr.Descr, "experiment") {
			continue
		}
		ss := strings.Split(pr.Descr, "\n")
		if len(ss) > 1 {
			prs[i].Descr = strings.Join(ss, " ")
		} else {
			prs[i].Descr = ss[0]
		}
		s := strings.TrimSpace(prs[i].Descr)
		prs[i].OrigDescr = s
		prs[i].Descr = sr.Replace(s)
		prs[i].Value, _ = strconv.Atoi(pr.Value)
	}
	return prs
}
