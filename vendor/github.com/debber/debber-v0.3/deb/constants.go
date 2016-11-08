package deb

import (
	"path/filepath"
)

const (
	//DebianBinaryVersionDefault is the current version as specified in .deb archives (filename debian-binary)
	DebianBinaryVersionDefault = "2.0"
	//DebianCompatDefault - compatibility. Current version
	DebianCompatDefault = "9"
	//FormatDefault - the format as specified in the dsc file (3.0 quilt uses a .debian.gz file rather than a .diff.gz file)
	FormatDefault = "3.0 (quilt)"
	//FormatQuilt - (3.0 quilt uses a .debian.gz file rather than a .diff.gz file)
	FormatQuilt = "3.0 (quilt)"
	//FormatNative - (3.0 native uses a .diff.gz file)
	FormatNative = "3.0 (native)"
	// StatusDefault is unreleased by default. Change this once you're happy with it.
	StatusDefault = "unreleased"

	//SectionDefault - devel seems to be the most common value
	SectionDefault = "devel"
	//PriorityDefault - 'extra' means 'low priority'
	PriorityDefault = "extra"
	//DependsDefault - No dependencies by default
	DependsDefault = ""
	//BuildDependsDefault - debhelper recommended for any package
	BuildDependsDefault = "debhelper (>= 9.1.0)"
	//BuildDependsGoDefault - golang required
	BuildDependsGoDefault = "debhelper (>= 9.1.0), golang-go"

	//StandardsVersionDefault - standards version is specified in the control file
	StandardsVersionDefault = "3.9.4"

	//ArchitectureDefault -'any' is the default architecture for source packages - not for binary debs
	ArchitectureDefault = "any"

	//TemplateDirDefault - the place where control file templates are kept
	TemplateDirDefault = "templates"
	//ResourcesDirDefault - the place where portable files are stored.
	ResourcesDirDefault = "resources"
	//WorkingDirDefault - the directory for build process.
	WorkingDirDefault = "."

	//ExeDirDefault - the default directory for exes within the data archive
	ExeDirDefault                   = "/usr/bin"
	BinaryDataArchiveNameDefault    = "data.tar.gz"
	BinaryControlArchiveNameDefault = "control.tar.gz"

	//OutDirDefault is the default output directory for temp or dist files
	outDirDefault = "target"
	DebianDir     = "debian"
)

const (
	PackageFName     = "Package"
	VersionFName     = "Version"
	DescriptionFName = "Description"
	MaintainerFName  = "Maintainer"

	ArchitectureFName = "Architecture" // Supported values: "all", "x386", "amd64", "armhf". TODO: armel

	DependsFName    = "Depends" // Depends
	RecommendsFName = "Recommends"
	SuggestsFName   = "Suggests"
	EnhancesFName   = "Enhances"
	PreDependsFName = "PreDepends"
	ConflictsFName  = "Conflicts"
	BreaksFName     = "Breaks"
	ProvidesFName   = "Provides"
	ReplacesFName   = "Replaces"

	BuildDependsFName      = "Build-Depends" // BuildDepends is only required for "sourcedebs".
	BuildDependsIndepFName = "Build-Depends-Indep"
	ConflictsIndepFName    = "Conflicts-Indep"
	BuiltUsingFName        = "Built-Using"

	PriorityFName         = "Priority"
	StandardsVersionFName = "Standards-Version"
	SectionFName          = "Section"
	FormatFName           = "Format"
	StatusFName           = "Status"
	OtherFName            = "Other"
	SourceFName           = "Source"
)

/*
Keyword	Meaning
public-domain	No license required for any purpose; the work is not subject to copyright in any jurisdiction.
Apache		Apache license 1.0, 2.0.
Artistic	Artistic license 1.0, 2.0.
BSD-2-clause	Berkeley software distribution license, 2-clause version.
BSD-3-clause	Berkeley software distribution license, 3-clause version.
BSD-4-clause	Berkeley software distribution license, 4-clause version.
ISC		Internet Software Consortium, sometimes also known as the OpenBSD License.
CC-BY		Creative Commons Attribution license 1.0, 2.0, 2.5, 3.0.
CC-BY-SA	Creative Commons Attribution Share Alike license 1.0, 2.0, 2.5, 3.0.
CC-BY-ND	Creative Commons Attribution No Derivatives license 1.0, 2.0, 2.5, 3.0.
CC-BY-NC	Creative Commons Attribution Non-Commercial license 1.0, 2.0, 2.5, 3.0.
CC-BY-NC-SA	Creative Commons Attribution Non-Commercial Share Alike license 1.0, 2.0, 2.5, 3.0.
CC-BY-NC-ND	Creative Commons Attribution Non-Commercial No Derivatives license 1.0, 2.0, 2.5, 3.0.
CC0		Creative Commons Zero 1.0 Universal. Omit "Universal" from the license version when forming the short name.
CDDL		Common Development and Distribution License 1.0.
CPL		IBM Common Public License.
EFL		The Eiffel Forum License 1.0, 2.0.
Expat		The Expat license.
GPL		GNU General Public License 1.0, 2.0, 3.0.
LGPL		GNU Lesser General Public License 2.1, 3.0, or GNU Library General Public License 2.0.
GFDL		GNU Free Documentation License 1.0, 1.1, 1.2, or 1.3. Use GFDL-NIV instead if there are no Front-Cover or Back-Cover Texts or Invariant Sections.
GFDL-NIV	GNU Free Documentation License, with no Front-Cover or Back-Cover Texts or Invariant Sections. Use the same version numbers as GFDL.
LPPL		LaTeX Project Public License 1.0, 1.1, 1.2, 1.3c.
MPL		Mozilla Public License 1.1.
Perl		Perl license (use "GPL-1+ or Artistic-1" instead).
Python		Python license 2.0.
QPL		Q Public License 1.0.
W3C		W3C Software License For more information, consult the W3C Intellectual Rights FAQ.
Zlib		zlib/libpng license.
Zope		Zope Public License 1.0, 1.1, 2.0, 2.1.

*/
var (
	Licenses = []string{
		"public-domain",
		"Apache",
		"Artistic",
		"BSD-2-clause",
		"BSD-3-clause",
		"BSD-4-clause",
		"ISC",
		"CC-BY",
		"CC-BY-SA",
		"CC-BY-ND",
		"CC-BY-NC",
		"CC-BY-NC-SA",
		"CC-BY-NC-ND",
		"CC0",
		"CDDL",
		"CPL",
		"EFL",
		"Expat",
		"GPL",
		"LGPL",
		"GFDL",
		"GFDL-NIV",
		"LPPL",
		"MPL",
		"Perl",
		"Python",
		"QPL",
		"W3C",
		"Zlib",
		"Zope",
	}
)

var (
	//TempDirDefault is the default directory for intermediate files
	TempDirDefault = filepath.Join(outDirDefault, "tmp")

	//DistDirDefault is the default directory for built artifacts
	DistDirDefault = outDirDefault

	MaintainerScripts = []string{"postinst", "postrm", "prerm", "preinst"}

	//SourceFields are the fields applicable to Source packages
	//
	// see http://manpages.ubuntu.com/manpages/precise/man5/deb-src-control.5.html://manpages.ubuntu.com/manpages/precise/man5/deb-src-control.5.html
	SourceFields = []string{
		SourceFName,
		MaintainerFName,
		"Uploaders",
		StandardsVersionFName,
		"DM-Upload-Allowed",
		"Homepage",
		"Bugs",
		"Vcs-Arch",
		"Vcs-Bzr",
		"Vcs-Cvs",
		"Vcs-Darcs",
		"Vcs-Git",
		"Vcs-Hg",
		"Vcs-Mtn",
		"Vcs-Svn",
		"Vcs-Browser",
		"Origin",
		SectionFName,
		PriorityFName,
		BuildDependsFName,
		"Build-Depends-Indep",
		"Build-Conflicts",
		"Build-Conflicts-Indep",
	}

	//BinaryFields are the fields applicable to binary packages
	//
	// see http://manpages.ubuntu.com/manpages/precise/man5/deb-src-control.5.html://manpages.ubuntu.com/manpages/precise/man5/deb-src-control.5.html
	BinaryFields = []string{
		PackageFName,
		ArchitectureFName,
		"Package-Type",
		"Subarchitecture",
		"Kernel-Version",
		"Installer-Menu-Item",
		"Essential",
		"Multi-Arch",
		"Tag",
		DescriptionFName,
		DependsFName,
		PreDependsFName,
		RecommendsFName,
		SuggestsFName,
		"Breaks",
		"Enhances",
		"Replaces",
		"Conflicts",
		"Provides",
		"Built-Using",
		PriorityFName,
		SectionFName,
		"Homepage",
	}
)
