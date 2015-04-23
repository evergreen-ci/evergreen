package debgen

import (
	"github.com/debber/debber-v0.3/deb"
)

const (
	GlobGoSources               = "*.go"
	TemplateDebianSourceFormat  = deb.FormatDefault                                      // Debian source formaat
	TemplateDebianSourceOptions = `tar-ignore = .hg
tar-ignore = .git
tar-ignore = .bzr` //specifies files to ignore while building.

	// The debian rules file describes how to build a 'source deb' into a binary deb. The default template here invokes debhelper scripts to automate this process for simple cases.
	TemplateDebianRulesDefault = `#!/usr/bin/make -f
# -*- makefile -*-

# Uncomment this to turn on verbose mode.
#export DH_VERBOSE=1

%:
	dh $@`

	// The debian rules file describes how to build a 'source deb' into a binary deb. This version contains a special script for building go packages.
	TemplateDebianRulesForGo = `#!/usr/bin/make -f
# -*- makefile -*-

# Uncomment this to turn on verbose mode.
#export DH_VERBOSE=1

export GOPATH=$(CURDIR){{range $i, $gpe := .ExtraData.GoPathExtra }}:{{$gpe}}{{end}}

PKGDIR=debian/{{.Package.Get "Package"}}

%:
	dh $@

clean:
	dh_clean
	rm -rf $(CURDIR)/bin/* $(CURDIR)/pkg/*
	rm -f $(CURDIR)/goinstall.log

binary-arch: clean
	dh_prep
	dh_installdirs
	cd $(CURDIR)/src && go install ./...

	mkdir -p $(PKGDIR)/usr/bin $(CURDIR)/bin/
	mkdir -p $(PKGDIR)/usr/share/gopkg/ $(CURDIR)/pkg/

	BINFILES=$(wildcard $(CURDIR)/bin/*)

	for x in$(BINFILES); do \
		cp $$x $(PKGDIR)/usr/bin/; \
	done;

	PKGFILES=$(wildcard $(CURDIR)/pkg/*.a)
	for x in$(PKGFILES); do \
		cp $$x $(PKGDIR)/usr/share/gopkg/; \
	done;

	dh_strip
	dh_compress
	dh_fixperms
	dh_installdeb
	dh_gencontrol
	dh_md5sums
	dh_builddeb

binary: binary-arch`

	// The debian control file (binary debs) defines package metadata
	TemplateBinarydebControl = `Package: {{.Package.Get "Package"}}
Priority: {{.Package.Get "Priority"}}
{{if .Package.Get "Maintainer"}}Maintainer: {{.Package.Get "Maintainer"}}
{{end}}Section: {{.Package.Get "Section"}}
Version: {{.Package.Get "Version"}}
Architecture: {{.Deb.Architecture}}
{{if .Package.Get "Depends"}}Depends: {{.Package.Get "Depends"}}
{{end}}Description: {{.Package.Get "Description"}}
`

	// The debian control file (source debs) defines general package metadata (but not version information)
	TemplateSnippetControlSourcePackage = `Source: {{.Get "Source"}}
Section: {{.Get "Section"}}
Priority: {{.Get "Priority"}}
Maintainer: {{.Get "Maintainer"}}
Build-Depends: {{.Get "Build-Depends"}}
Standards-Version: {{.Get "Standards-Version"}}`

	TemplateSnippetControlBinPackage = `Package: {{.Get "Package"}}
Architecture: {{.Get "Architecture"}}
{{if .Get "Depends"}}Depends: {{.Get "Depends"}}
{{end}}Description: {{.Get "Description"}}`

	TemplateSourcedebControl = `{{range .Package}}{{if .Get "Source"}}` + TemplateSnippetControlSourcePackage + `{{end}}{{if .Get "Package"}}` + TemplateSnippetControlBinPackage + `{{end}}

{{end}}`

	TemplateSnippetSourcedebControlDevPackage = `Package: {{.Package.Get "Package"}}-dev
Architecture: {{.Package.Get "Architecture"}}
{{if .Package.Get "Depends"}}Depends: {{.Package.Get "Depends"}}
{{end}}Description: {{.Package.Get "Description"}}
Section: libdevel
{{.Package.Get "Other"}}`

	/*
		TemplateSourcedebControl = TemplateSnippetSourcedebControl + "\n\n" + TemplateSnippetSourcedebControlPackage
		TemplateSourcedebWithDevControl = TemplateSnippetSourcedebControl + "\n\n" + TemplateSnippetSourcedebControlPackage + "\n\n" + TemplateSnippetSourcedebControlDevPackage
	*/
	// The dsc file defines package metadata AND checksums
	TemplateDebianDsc = `Format: {{.Package.Get "Format"}}
Source: {{.Package.Get "Source"}}
Binary: {{.Package.Get "Package"}}
Architecture: {{.Package.Get "Architecture"}}
Version: {{.Package.Get "Version"}}
Maintainer: {{.Package.Get "Maintainer"}}
{{if .Package.Get "Uploaders"}}Uploaders: {{.Package.Get "Uploaders"}}
{{end}}Standards-Version: {{.Package.Get "Standards-Version"}}
Build-Depends: {{.Package.Get "Build-Depends"}}
Priority: {{.Package.Get "Priority"}}
Section: {{.Package.Get "Section"}}
Checksums-Sha1:{{range .Checksums.ChecksumsSha1}}
 {{.Checksum}} {{.Size}} {{.File}}{{end}}
Checksums-Sha256:{{range .Checksums.ChecksumsSha256}}
 {{.Checksum}} {{.Size}} {{.File}}{{end}}
Files:{{range .Checksums.ChecksumsMd5}}
 {{.Checksum}} {{.Size}} {{.File}}{{end}}
{{.Package.Get "Other"}}`

	TemplateChangelogHeader          = `{{.Package.Get "Package"}} ({{.Package.Get "Version"}}) {{.Package.Get "Status"}}; urgency=low`
	TemplateChangelogInitialEntry    = `  * Initial import`
	TemplateChangelogFooter          = ` -- {{.Package.Get "Maintainer"}}  {{.EntryDate}}`
	TemplateChangelogInitial         = TemplateChangelogHeader + "\n\n" + TemplateChangelogInitialEntry + "\n\n" + TemplateChangelogFooter
	TemplateChangelogAdditionalEntry = "\n\n" + TemplateChangelogHeader + "\n\n{{.ChangelogEntry}}\n\n" + TemplateChangelogFooter
	TemplateDebianCopyright          = `Copyright 2014 {{.Package.Get "Package"}}`
	TemplateDebianReadme             = `{{.Package.Get "Package"}}
==========

`
	TemplateCopyrightBasic = `Files: *
Copyright: {{.ExtraData.Year}} {{.Package.Get "Maintainer"}}
License: {{.ExtraData.License}}
`
	DevGoPathDefault   = "/usr/share/gocode" // This is used by existing -dev.deb packages e.g. golang-doozer-dev and golang-protobuf-dev
	GoPathExtraDefault = ":" + DevGoPathDefault

	DebianDir    = "debian"
	TplExtension = ".tpl"

	ChangelogDateLayout = "Mon, 02 Jan 2006 15:04:05 -0700"
)

var (
	TemplateStringsSourceDefault = map[string]string{
		"control":        TemplateSourcedebControl,
		"compat":         deb.DebianCompatDefault,
		"rules":          TemplateDebianRulesDefault,
		"source/format":  TemplateDebianSourceFormat,
		"source/options": TemplateDebianSourceOptions,
		"copyright":      TemplateDebianCopyright,
		"changelog":      TemplateChangelogInitial,
		"README.debian":  TemplateDebianReadme}
)
