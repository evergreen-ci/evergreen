package model

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper/testutil"
	"github.com/mongodb/jasper/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGolangVariantPackage(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("FailsWithoutRef", func(t *testing.T) {
			gvp := GolangVariantPackage{}
			assert.Error(t, gvp.Validate())
		})
		t.Run("SucceedsIfNameSet", func(t *testing.T) {
			gvp := GolangVariantPackage{Name: "name"}
			assert.NoError(t, gvp.Validate())
		})
		t.Run("SucceedsIfPathSet", func(t *testing.T) {
			gvp := GolangVariantPackage{Path: "path"}
			assert.NoError(t, gvp.Validate())
		})
		t.Run("SucceedsIfTagSet", func(t *testing.T) {
			gvp := GolangVariantPackage{Tag: "tag"}
			assert.NoError(t, gvp.Validate())
		})
		t.Run("FailsIfNameAndPathSet", func(t *testing.T) {
			gvp := GolangVariantPackage{
				Name: "name",
				Path: "path",
			}
			assert.Error(t, gvp.Validate())
		})
		t.Run("FailsIfNameAndTagSet", func(t *testing.T) {
			gvp := GolangVariantPackage{
				Name: "name",
				Tag:  "tag",
			}
			assert.Error(t, gvp.Validate())
		})
		t.Run("FailsIfPathAndTagSet", func(t *testing.T) {
			gvp := GolangVariantPackage{
				Path: "path",
				Tag:  "tag",
			}
			assert.Error(t, gvp.Validate())
		})
		t.Run("FailsIfAllSet", func(t *testing.T) {
			gvp := GolangVariantPackage{
				Name: "name",
				Path: "path",
				Tag:  "tag",
			}
			assert.Error(t, gvp.Validate())
		})
	})
}

func TestGolangVariant(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		for testName, testCase := range map[string]func(t *testing.T, v *GolangVariant){
			"Succeeds": func(t *testing.T, gv *GolangVariant) {
				assert.NoError(t, gv.Validate())
			},
			"FailsWithoutName": func(t *testing.T, gv *GolangVariant) {
				gv.Name = ""
				assert.Error(t, gv.Validate())
			},
			"FailsWithoutDistros": func(t *testing.T, gv *GolangVariant) {
				gv.Distros = nil
				assert.Error(t, gv.Validate())
			},
			"FailsWithoutPackages": func(t *testing.T, gv *GolangVariant) {
				gv.Packages = nil
				assert.Error(t, gv.Validate())
			},
			"FailsWithInvalidPackage": func(t *testing.T, gv *GolangVariant) {
				gv.Packages = []GolangVariantPackage{{}}
				assert.Error(t, gv.Validate())
			},
			"FailsWithDuplicatePackageName": func(t *testing.T, gv *GolangVariant) {
				gv.Packages = []GolangVariantPackage{
					{Name: "name"},
					{Name: "name"},
				}
				assert.Error(t, gv.Validate())
			},
			"FailsWithDuplicatePackagePath": func(t *testing.T, gv *GolangVariant) {
				gv.Packages = []GolangVariantPackage{
					{Path: "path"},
					{Path: "path"},
				}
				assert.Error(t, gv.Validate())
			},
			"FailsWithDuplicatePackageTag": func(t *testing.T, gv *GolangVariant) {
				gv.Packages = []GolangVariantPackage{
					{Tag: "tag"},
					{Tag: "tag"},
				}
				assert.Error(t, gv.Validate())
			},
		} {
			t.Run(testName, func(t *testing.T) {
				gv := GolangVariant{
					VariantDistro: VariantDistro{
						Name:    "var_name",
						Distros: []string{"distro1", "distro2"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Name: "name"},
							{Path: "path"},
							{Tag: "tag"},
						},
					},
				}
				testCase(t, &gv)
			})
		}
	})
}

func TestGolangVariantParameters(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("SucceedsWithName", func(t *testing.T) {
			gvp := GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Name: "name"},
				},
			}
			assert.NoError(t, gvp.Validate())
		})
		t.Run("SucceedsWithPath", func(t *testing.T) {
			gvp := GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Path: "path"},
				},
			}
			assert.NoError(t, gvp.Validate())
		})
		t.Run("SucceedsWithTag", func(t *testing.T) {
			gvp := GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Tag: "tag"},
				},
			}
			assert.NoError(t, gvp.Validate())
		})
		t.Run("SucceedsWithMultiple", func(t *testing.T) {
			gvp := GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Name: "name1"},
					{Name: "name2"},
					{Path: "path1"},
					{Path: "path2"},
					{Tag: "tag1"},
					{Tag: "tag2"},
				},
			}
			assert.NoError(t, gvp.Validate())
		})
		t.Run("FailsWithEmpty", func(t *testing.T) {
			gvp := GolangVariantParameters{}
			assert.Error(t, gvp.Validate())
		})
		t.Run("FailsWithDuplicateName", func(t *testing.T) {
			gvp := GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Name: "name"},
					{Name: "name"},
				},
			}
			assert.Error(t, gvp.Validate())
		})
		t.Run("FailsWithDuplicatePath", func(t *testing.T) {
			gvp := GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Path: "path"},
					{Path: "path"},
				},
			}
			assert.Error(t, gvp.Validate())
		})
		t.Run("FailsWithInvalidOptions", func(t *testing.T) {
			opts := GolangRuntimeOptions([]string{"-v"})
			gvp := GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Name: "name"},
				},
				Options: &opts,
			}
			assert.Error(t, gvp.Validate())
		})
	})
}

func TestNamedGolangVariantParameters(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("Succeeds", func(t *testing.T) {
			ngvp := NamedGolangVariantParameters{
				Name: "variant",
				GolangVariantParameters: GolangVariantParameters{
					Packages: []GolangVariantPackage{
						{Name: "package"},
					},
				},
			}
			assert.NoError(t, ngvp.Validate())
		})
		t.Run("FailsWithEmpty", func(t *testing.T) {
			ngvp := NamedGolangVariantParameters{}
			assert.Error(t, ngvp.Validate())
		})
		t.Run("FailsWithoutName", func(t *testing.T) {
			ngvp := NamedGolangVariantParameters{
				GolangVariantParameters: GolangVariantParameters{
					Packages: []GolangVariantPackage{
						{Name: "package"},
					},
				},
			}
			assert.Error(t, ngvp.Validate())
		})
		t.Run("FailsWithInvalidParameters", func(t *testing.T) {
			ngvp := NamedGolangVariantParameters{
				Name: "variant",
			}
			assert.Error(t, ngvp.Validate())
		})
	})
}

func TestGolangPackage(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		for testName, testCase := range map[string]func(t *testing.T, gp *GolangPackage){
			"Succeeds": func(t *testing.T, gp *GolangPackage) {
				assert.NoError(t, gp.Validate())
			},
			"FailsWithoutPath": func(t *testing.T, gp *GolangPackage) {
				gp.Path = ""
				assert.Error(t, gp.Validate())
			},
			"FailsWithDuplicateTags": func(t *testing.T, gp *GolangPackage) {
				gp.Tags = []string{"tag1", "tag1"}
			},
		} {
			t.Run(testName, func(t *testing.T) {
				gp := GolangPackage{
					Name: "name",
					Path: "path",
					Tags: []string{"tag1"},
				}
				testCase(t, &gp)
			})
		}
	})
}

func TestGolangGetPackageIndexByName(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"Succeeds": func(t *testing.T, g *Golang) {
			gp, i, err := g.GetPackageIndexByName("package1")
			require.NoError(t, err)
			assert.Equal(t, 0, i)
			assert.Equal(t, "package1", gp.Name)
		},
		"FailsIfPackageNotFound": func(t *testing.T, g *Golang) {
			gp, i, err := g.GetPackageIndexByName("foo")
			assert.Error(t, err)
			assert.Equal(t, -1, i)
			assert.Zero(t, gp)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Packages: []GolangPackage{
					{Name: "package1"},
					{Name: "package2"},
				},
			}
			testCase(t, &g)
		})
	}
}

func TestGolangGetUnnamedPackagesByPath(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"Succeeds": func(t *testing.T, g *Golang) {
			gp, i, err := g.GetUnnamedPackageIndexByPath("path1")
			require.NoError(t, err)
			assert.Equal(t, 0, i)
			assert.Equal(t, "path1", gp.Path)
		},
		"FailsIfPackageNotFound": func(t *testing.T, g *Golang) {
			gp, i, err := g.GetUnnamedPackageIndexByPath("")
			assert.Error(t, err)
			assert.Equal(t, -1, i)
			assert.Zero(t, gp)
		},
		"FailsIfNamedPackageWithPath": func(t *testing.T, g *Golang) {
			gp, i, err := g.GetUnnamedPackageIndexByPath("path3")
			assert.Error(t, err)
			assert.Equal(t, -1, i)
			assert.Zero(t, gp)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Packages: []GolangPackage{
					{Path: "path1"},
					{Path: "path2"},
					{Name: "name3", Path: "path3"},
				},
			}
			testCase(t, &g)
		})
	}
}

func TestGolangGetPackagesByTag(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"Succeeds": func(t *testing.T, g *Golang) {
			gps := g.GetPackagesByTag("tag2")
			require.Len(t, gps, 1)
			assert.Equal(t, "path1", gps[0].Path)
		},
		"FailsIfPackageNotFound": func(t *testing.T, g *Golang) {
			gps := g.GetPackagesByTag("foo")
			assert.Empty(t, gps)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Packages: []GolangPackage{
					{Path: "path1", Tags: []string{"tag1", "tag2"}},
					{Path: "path2", Tags: []string{"tag1", "tag3"}},
				},
			}
			testCase(t, &g)
		})
	}
}

func TestGolangGetPackagesAndRef(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"SucceedsWithName": func(t *testing.T, g *Golang) {
			gps, ref, err := g.GetPackagesAndRef(GolangVariantPackage{Name: "package1"})
			require.NoError(t, err)
			require.Len(t, gps, 1)
			assert.Equal(t, "package1", ref)
			assert.Equal(t, g.Packages[0], gps[0])
		},
		"SucceedsWithPath": func(t *testing.T, g *Golang) {
			gps, ref, err := g.GetPackagesAndRef(GolangVariantPackage{Path: "path1"})
			require.NoError(t, err)
			require.Len(t, gps, 1)
			assert.Equal(t, "path1", ref)
			assert.Equal(t, g.Packages[1], gps[0])
		},
		"SucceedsWithTag": func(t *testing.T, g *Golang) {
			gps, ref, err := g.GetPackagesAndRef(GolangVariantPackage{Tag: "tag"})
			require.NoError(t, err)
			require.Len(t, gps, 2)
			assert.Equal(t, "tag", ref)
			assert.Equal(t, g.Packages[0], gps[0])
		},
		"FailsWithEmpty": func(t *testing.T, g *Golang) {
			gps, ref, err := g.GetPackagesAndRef(GolangVariantPackage{})
			assert.Error(t, err)
			assert.Zero(t, ref)
			assert.Zero(t, gps)
		},
		"FailsWithUnmatchedName": func(t *testing.T, g *Golang) {
			gps, ref, err := g.GetPackagesAndRef(GolangVariantPackage{Name: "foo"})
			assert.Error(t, err)
			assert.Zero(t, ref)
			assert.Zero(t, gps)
		},
		"FailsWithUnmatchedPath": func(t *testing.T, g *Golang) {
			gps, ref, err := g.GetPackagesAndRef(GolangVariantPackage{Path: "foo"})
			assert.Error(t, err)
			assert.Zero(t, ref)
			assert.Zero(t, gps)
		},
		"FailsWithUnmatchedTag": func(t *testing.T, g *Golang) {
			gps, ref, err := g.GetPackagesAndRef(GolangVariantPackage{Tag: "foo"})
			assert.Error(t, err)
			assert.Zero(t, ref)
			assert.Zero(t, gps)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Packages: []GolangPackage{
					{Name: "package1", Path: "path1", Tags: []string{"tag"}},
					{Path: "path1"},
					{Name: "package2", Path: "path2", Tags: []string{"tag"}},
				},
			}
			testCase(t, &g)
		})
	}
}

func TestGolangGetVariantIndexByName(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"Succeeds": func(t *testing.T, g *Golang) {
			gv, i, err := g.GetVariantIndexByName("variant")
			require.NoError(t, err)
			assert.Equal(t, 0, i)
			assert.Equal(t, "variant", gv.Name)
		},
		"FailsIfVariantNotFound": func(t *testing.T, g *Golang) {
			gv, i, err := g.GetVariantIndexByName("foo")
			assert.Error(t, err)
			assert.Equal(t, -1, i)
			assert.Zero(t, gv)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Variants: []GolangVariant{
					{VariantDistro: VariantDistro{Name: "variant"}},
				},
			}
			testCase(t, &g)
		})
	}
}

func TestGolangValidate(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"Succeeds": func(t *testing.T, g *Golang) {
			assert.NoError(t, g.Validate())
		},
		"FailsWithoutRootPackage": func(t *testing.T, g *Golang) {
			g.RootPackage = ""
			assert.Error(t, g.Validate())
		},
		"SucceedsWithGOROOTInEnvironment": func(t *testing.T, g *Golang) {
			goroot := os.Getenv("GOROOT")
			if goroot == "" {
				t.Skip("GOROOT is not defined in environment")
			}
			delete(g.Environment, "GOROOT")
			assert.NoError(t, g.Validate())
			assert.Equal(t, goroot, g.Environment["GOROOT"])
		},
		"FailsWithoutGOROOTEnvVar": func(t *testing.T, g *Golang) {
			if goroot, ok := os.LookupEnv("GOROOT"); ok {
				defer func() {
					os.Setenv("GOROOT", goroot)
				}()
				require.NoError(t, os.Unsetenv("GOROOT"))
			}
			delete(g.Environment, "GOROOT")
			assert.Error(t, g.Validate())
		},
		"SucceedsIfGOPATHInEnvironmentAndIsWithinWorkingDirectory": func(t *testing.T, g *Golang) {
			gopath := os.Getenv("GOPATH")
			if gopath == "" {
				t.Skip("GOPATH not defined in environment")
			}
			g.WorkingDirectory = util.ConsistentFilepath(filepath.Dir(gopath))
			delete(g.Environment, "GOPATH")
			assert.NoError(t, g.Validate())
			relGopath, err := filepath.Rel(g.WorkingDirectory, gopath)
			require.NoError(t, err)
			assert.Equal(t, util.ConsistentFilepath(relGopath), util.ConsistentFilepath(g.Environment["GOPATH"]))
		},
		"FailsWithoutGOPATHEnvVar": func(t *testing.T, g *Golang) {
			if gopath, ok := os.LookupEnv("GOPATH"); ok {
				defer func() {
					os.Setenv("GOPATH", gopath)
				}()
				require.NoError(t, os.Unsetenv("GOPATH"))
			}
			delete(g.Environment, "GOPATH")
			assert.Error(t, g.Validate())
		},
		"FailsWithoutPackages": func(t *testing.T, g *Golang) {
			g.Packages = nil
			assert.Error(t, g.Validate())
		},
		"FailsWithInvalidPackage": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{{}}
			assert.Error(t, g.Validate())
		},
		"SucceedsWithUniquePackageNames": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Path: "path1"},
				{Name: "name2", Path: "path2"},
			}
			assert.NoError(t, g.Validate())
		},
		"FailsWithDuplicatePackageName": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Name: "name", Path: "path1"},
				{Name: "name", Path: "path2"},
			}
			assert.Error(t, g.Validate())
		},
		"SucceedsWithUniquePackagePaths": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Path: "path1"},
				{Path: "path2"},
			}
			assert.NoError(t, g.Validate())
		},
		"SucceedsWithDuplicatePackagePathButUniqueNames": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Name: "name1", Path: "path1"},
				{Name: "name2", Path: "path1"},
			}
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Name: "name1"},
							{Name: "name2"},
						},
					},
				},
			}
			assert.NoError(t, g.Validate())
		},
		"FailsWithDuplicateUnnamedPackagePath": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Path: "path"},
				{Path: "path"},
			}
			assert.Error(t, g.Validate())
		},
		"FailsWithPackageNameMatchingUnnamedPackagePath": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Name: "path", Path: "path"},
				{Path: "path"},
			}
			assert.Error(t, g.Validate())
		},
		"FailsWithoutVariants": func(t *testing.T, g *Golang) {
			g.Variants = nil
			assert.Error(t, g.Validate())
		},
		"FailsWithInvalidVariant": func(t *testing.T, g *Golang) {
			g.Variants = []GolangVariant{{}}
			assert.Error(t, g.Validate())
		},
		"FailsWithDuplicateVariantName": func(t *testing.T, g *Golang) {
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Path: "path1"},
						},
					},
				},
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Path: "path1"},
						},
					},
				},
			}
			assert.Error(t, g.Validate())
		},
		"SucceedsWithValidVariantPackageName": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Name: "name", Path: "path"},
			}
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Name: "name"},
						},
					},
				},
			}
			assert.NoError(t, g.Validate())
		},
		"FailsWithInvalidVariantPackageName": func(t *testing.T, g *Golang) {
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Name: "nonexistent"},
						},
					},
				},
			}
			assert.Error(t, g.Validate())
		},
		"SucceedsWithValidVariantPackagePath": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Path: "path"},
			}
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Path: "path"},
						},
					},
				},
			}
			assert.NoError(t, g.Validate())
		},
		"FailsWithDuplicateGolangPackageReferences": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Path: "path", Tags: []string{"tag"}},
			}
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Path: "path"},
							{Tag: "tag"},
						},
					},
				},
			}
			assert.Error(t, g.Validate())
		},
		"FailsWithInvalidVariantPackagePath": func(t *testing.T, g *Golang) {
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Path: "nonexistent"},
						},
					},
				},
			}
			assert.Error(t, g.Validate())
		},
		"SucceedsWithValidVariantPackageTag": func(t *testing.T, g *Golang) {
			g.Packages = []GolangPackage{
				{Path: "path", Tags: []string{"tag"}},
			}
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Tag: "tag"},
						},
					},
				},
			}
			assert.NoError(t, g.Validate())
		},
		"FailsWithInvalidVariantPackageTag": func(t *testing.T, g *Golang) {
			g.Variants = []GolangVariant{
				{
					VariantDistro: VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: GolangVariantParameters{
						Packages: []GolangVariantPackage{
							{Tag: "nonexistent"},
						},
					},
				},
			}
			assert.Error(t, g.Validate())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				RootPackage: "root_package",
				Environment: map[string]string{
					"GOPATH": "gopath",
					"GOROOT": "goroot",
				},
				Packages: []GolangPackage{
					{Path: "path1"},
					{Path: "path2"},
				},
				Variants: []GolangVariant{
					{
						VariantDistro: VariantDistro{
							Name:    "variant",
							Distros: []string{"distro"},
						},
						GolangVariantParameters: GolangVariantParameters{
							Packages: []GolangVariantPackage{
								{Path: "path1"},
							},
						},
					},
				},
			}
			testCase(t, &g)
		})
	}
}

func TestDiscoverPackages(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, g *Golang, rootPath string){
		"FailsIfPackageNotFound": func(t *testing.T, g *Golang, rootPath string) {
			g.RootPackage = "foo"
			assert.Error(t, g.DiscoverPackages())
		},
		"DoesNotDiscoverPackageWithoutTestFiles": func(t *testing.T, g *Golang, rootPath string) {
			assert.NoError(t, g.DiscoverPackages())
			assert.Empty(t, g.Packages)
		},
		"DiscoversPackageIfTestFilesPresent": func(t *testing.T, g *Golang, rootPath string) {
			f, err := os.Create(filepath.Join(rootPath, "fake_test.go"))
			require.NoError(t, err)
			require.NoError(t, f.Close())

			assert.NoError(t, g.DiscoverPackages())
			require.Len(t, g.Packages, 1)
			assert.Equal(t, ".", g.Packages[0].Path)
			assert.Empty(t, g.Packages[0].Name)
			assert.Empty(t, g.Packages[0].Tags)
		},
		"DoesNotModifyPackageDefinitionIfAlreadyDefined": func(t *testing.T, g *Golang, rootPath string) {
			gp := GolangPackage{
				Name: "package_name",
				Path: ".",
				Tags: []string{"tag"},
			}
			g.Packages = []GolangPackage{gp}
			f, err := os.Create(filepath.Join(rootPath, "fake_test.go"))
			require.NoError(t, err)
			require.NoError(t, f.Close())

			assert.NoError(t, g.DiscoverPackages())
			require.Len(t, g.Packages, 1)
			assert.Equal(t, gp.Path, g.Packages[0].Path)
			assert.Equal(t, gp.Name, g.Packages[0].Name)
			assert.Equal(t, gp.Tags, g.Packages[0].Tags)
		},
		"IgnoresVendorDirectory": func(t *testing.T, g *Golang, rootPath string) {
			vendorDir := filepath.Join(rootPath, golangVendorDir)
			require.NoError(t, os.Mkdir(vendorDir, 0777))
			f, err := os.Create(filepath.Join(vendorDir, "fake_test.go"))
			require.NoError(t, err)
			require.NoError(t, f.Close())

			assert.NoError(t, g.DiscoverPackages())
			assert.Empty(t, g.Packages)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			rootPackage := util.ConsistentFilepath("github.com", "fake_user", "fake_repo")
			gopath, err := ioutil.TempDir(testutil.BuildDirectory(), "gopath")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(gopath))
			}()
			rootPath := util.ConsistentFilepath(gopath, "src", rootPackage)
			require.NoError(t, os.MkdirAll(rootPath, 0777))

			g := Golang{
				Environment: map[string]string{
					"GOPATH": gopath,
					"GOROOT": "some_goroot",
				},
				RootPackage:      rootPackage,
				WorkingDirectory: util.ConsistentFilepath(filepath.Dir(gopath)),
			}
			testCase(t, &g, rootPath)
		})
	}
}

func TestGolangRuntimeOptions(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			opts        GolangRuntimeOptions
			expectError bool
		}{
			"SucceedsWithAllUniqueFlags": {
				opts: []string{"-cover", "-coverprofile", "-race"},
			},
			"FailsWithDuplicateFlags": {
				opts:        []string{"-race", "-race"},
				expectError: true,
			},
			"FailsWithDuplicateEquivalentFlags": {
				opts:        []string{"-race", "-test.race"},
				expectError: true,
			},
			"FailsWithVerboseFlag": {
				opts:        []string{"-v"},
				expectError: true,
			},
		} {
			t.Run(testName, func(t *testing.T) {
				err := testCase.opts.Validate()
				if testCase.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
	t.Run("Merge", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			opts      GolangRuntimeOptions
			overwrite GolangRuntimeOptions
			expected  GolangRuntimeOptions
		}{
			"AllUniqueFlagsAreAppended": {
				opts:      []string{"-cover", "-race=true"},
				overwrite: []string{"-coverprofile", "-outputdir=./dir"},
				expected:  []string{"-cover", "-race=true", "-coverprofile", "-outputdir=./dir"},
			},
			"DuplicateFlagsAreCombined": {
				opts:      []string{"-cover"},
				overwrite: []string{"-cover"},
				expected:  []string{"-cover"},
			},
			"TestFlagsAreCheckedAgainstEquivalentFlags": {
				opts:      []string{"-test.race"},
				overwrite: []string{"-race"},
				expected:  []string{"-race"},
			},
			"ConflictingTestFlagsAreOverwritten": {
				opts:      []string{"-test.race=true"},
				overwrite: []string{"-test.race=false"},
				expected:  []string{"-test.race=false"},
			},
			"UniqueFlagsAreAppendedAndDuplicateFlagsAreCombined": {
				opts:      []string{"-cover"},
				overwrite: []string{"-cover", "-coverprofile"},
				expected:  []string{"-cover", "-coverprofile"},
			},
			"ConflictingFlagValuesAreOverwritten": {
				opts:      []string{"-race=false"},
				overwrite: []string{"-race=true"},
				expected:  []string{"-race=true"},
			},
			"UniqueFlagsAreAppendedAndConflictingFlagsAreOverwritten": {
				opts:      []string{"-cover", "-race=false"},
				overwrite: []string{"-race=true"},
				expected:  []string{"-cover", "-race=true"},
			},
			"DuplicateFlagsAreCombinedAndConflictingFlagsAreOverwritten": {
				opts:      []string{"-cover", "-race=false"},
				overwrite: []string{"-cover", "-race=true"},
				expected:  []string{"-cover", "-race=true"},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				merged := testCase.opts.Merge(testCase.overwrite)
				assert.Len(t, merged, len(testCase.expected))
				for _, flag := range merged {
					assert.True(t, utility.StringSliceContains(testCase.expected, flag))
				}
			})
		}

	})
}

func TestGolangMergePackages(t *testing.T) {
	gps := []GolangPackage{
		{
			Name: "package1",
			Path: "path1",
		},
		{
			Path: "path1",
		},
	}

	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"OverwritesExistingPackageWithMatchingName": func(t *testing.T, g *Golang) {
			gp := GolangPackage{
				Name: "package1",
				Path: "path2",
				Tags: []string{"tag1"},
			}
			_ = g.MergePackages(gp)
			require.Len(t, g.Packages, 2)
			assert.Equal(t, gp, g.Packages[0])
			assert.Equal(t, gps[1], g.Packages[1])
		},
		"OverwritesExistingUnnamedPackageWithMatchingPath": func(t *testing.T, g *Golang) {
			gp := GolangPackage{
				Path: "path1",
				Tags: []string{"tag1"},
			}
			_ = g.MergePackages(gp)
			require.Len(t, g.Packages, 2)
			assert.Equal(t, gps[0], g.Packages[0])
			assert.Equal(t, gp, g.Packages[1])
		},
		"AddsNewNamedPackage": func(t *testing.T, g *Golang) {
			gp := GolangPackage{
				Name: "package2",
				Path: "path1",
				Tags: []string{"tag1"},
			}
			_ = g.MergePackages(gp)
			require.Len(t, g.Packages, 3)
			assert.Equal(t, gps[0:2], g.Packages[0:2])
			assert.Equal(t, gp, g.Packages[2])
		},
		"AddsNewUnnamedPackage": func(t *testing.T, g *Golang) {
			gp := GolangPackage{
				Name: "package2",
				Path: "path2",
				Tags: []string{"tag1"},
			}
			_ = g.MergePackages(gp)
			require.Len(t, g.Packages, 3)
			assert.Equal(t, gps[0:2], g.Packages[0:2])
			assert.Equal(t, gp, g.Packages[2])
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Packages: gps,
			}
			testCase(t, &g)
		})
	}
}

func TestGolangMergeVariantDistros(t *testing.T) {
	gvs := []GolangVariant{
		{
			VariantDistro: VariantDistro{
				Name:    "variant1",
				Distros: []string{"distro1"},
			},
		},
		{
			VariantDistro: VariantDistro{
				Name:    "variant2",
				Distros: []string{"distro2"},
			},
		},
	}

	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"OverwritesExistingWithMatchingName": func(t *testing.T, g *Golang) {
			vd := VariantDistro{
				Name:    "variant1",
				Distros: []string{"distro3"},
			}
			_ = g.MergeVariantDistros(vd)
			require.Len(t, g.Variants, 2)
			assert.Equal(t, vd, g.Variants[0].VariantDistro)
			assert.Equal(t, gvs[1], g.Variants[1])
		},
		"AddsNewVariant": func(t *testing.T, g *Golang) {
			vd := VariantDistro{
				Name:    "variant3",
				Distros: []string{"distro3"},
			}
			_ = g.MergeVariantDistros(vd)
			require.Len(t, g.Variants, 3)
			assert.Equal(t, gvs[0:2], g.Variants[0:2])
			assert.Equal(t, vd, g.Variants[2].VariantDistro)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Variants: gvs,
			}
			testCase(t, &g)
		})
	}
}

func TestGolangMergeVariantParameters(t *testing.T) {
	gvs := []GolangVariant{
		{
			VariantDistro: VariantDistro{
				Name: "variant1",
			},
			GolangVariantParameters: GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Name: "package1"},
				},
			},
		},
		{
			VariantDistro: VariantDistro{
				Name: "variant2",
			},
			GolangVariantParameters: GolangVariantParameters{
				Packages: []GolangVariantPackage{
					{Name: "package2"},
				},
			},
		},
	}

	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"OverwritesExistingWithMatchingName": func(t *testing.T, g *Golang) {
			ngvp := NamedGolangVariantParameters{
				Name: "variant1",
				GolangVariantParameters: GolangVariantParameters{
					Packages: []GolangVariantPackage{
						{Name: "package3"},
					},
				},
			}
			_ = g.MergeVariantParameters(ngvp)
			require.Len(t, g.Variants, 2)
			assert.Equal(t, ngvp.GolangVariantParameters, g.Variants[0].GolangVariantParameters)
			assert.Equal(t, gvs[1], g.Variants[1])
		},
		"AddsNewVariant": func(t *testing.T, g *Golang) {
			ngvp := NamedGolangVariantParameters{
				Name: "variant3",
				GolangVariantParameters: GolangVariantParameters{
					Packages: []GolangVariantPackage{
						{Name: "package3"},
					},
				},
			}
			_ = g.MergeVariantParameters(ngvp)
			require.Len(t, g.Variants, 3)
			assert.Equal(t, gvs[0:2], g.Variants[0:2])
			assert.Equal(t, ngvp.GolangVariantParameters, g.Variants[2].GolangVariantParameters)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Variants: gvs,
			}
			testCase(t, &g)
		})
	}
}

func TestGolangMergeEnvironments(t *testing.T) {
	env := map[string]string{
		"key1": "val1",
		"key2": "val2",
	}
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"OverwritesExistingWithMatchingName": func(t *testing.T, g *Golang) {
			newEnv := map[string]string{
				"key1": "val3",
			}
			_ = g.MergeEnvironments(newEnv)
			assert.Len(t, g.Environment, 2)
			assert.Equal(t, newEnv["key1"], g.Environment["key1"])
			assert.Equal(t, env["key2"], g.Environment["key2"])
		},
		"AddsNewEnvVars": func(t *testing.T, g *Golang) {
			newEnv := map[string]string{
				"key3": "val3",
			}
			_ = g.MergeEnvironments(newEnv)
			assert.Len(t, g.Environment, 3)
			assert.Equal(t, env["key1"], g.Environment["key1"])
			assert.Equal(t, env["key2"], g.Environment["key2"])
			assert.Equal(t, newEnv["key3"], g.Environment["key3"])
		},
	} {
		t.Run(testName, func(t *testing.T) {
			g := Golang{
				Environment: env,
			}
			testCase(t, &g)
		})
	}
}

func TestGolangMergeDefaultTags(t *testing.T) {
	defaultTags := []string{"tag"}
	for testName, testCase := range map[string]func(t *testing.T, m *Golang){
		"AddsNewTags": func(t *testing.T, m *Golang) {
			_ = m.MergeDefaultTags("newTag1", "newTag2")
			assert.Len(t, m.DefaultTags, len(defaultTags)+2)
			assert.Subset(t, m.DefaultTags, defaultTags)
			assert.Contains(t, m.DefaultTags, "newTag1")
			assert.Contains(t, m.DefaultTags, "newTag2")
		},
		"IgnoresDuplicateTags": func(t *testing.T, m *Golang) {
			_ = m.MergeDefaultTags("tag")
			assert.Len(t, m.DefaultTags, len(defaultTags))
			assert.Subset(t, m.DefaultTags, defaultTags)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			m := Golang{
				DefaultTags: defaultTags,
			}
			testCase(t, &m)
		})
	}
}

func TestGolangApplyDefaultTags(t *testing.T) {
	defaultTags := []string{"default_tag1", "default_tag2"}
	for testName, testCase := range map[string]func(t *testing.T, m *Golang){
		"AddsNewDefaultTags": func(t *testing.T, m *Golang) {
			tags := []string{"tag"}
			m.Packages = []GolangPackage{
				{
					Name: "task",
					Tags: tags,
				},
			}
			m.ApplyDefaultTags()
			assert.Len(t, m.Packages[0].Tags, len(tags)+len(defaultTags))
			assert.Subset(t, m.Packages[0].Tags, tags)
			assert.Subset(t, m.Packages[0].Tags, defaultTags)
		},
		"IgnoresTagsThatAlreadyExist": func(t *testing.T, m *Golang) {
			tags := append([]string{"tag"}, defaultTags...)
			m.Packages = []GolangPackage{
				{
					Name: "task",
					Tags: tags,
				},
			}
			m.ApplyDefaultTags()
			assert.Len(t, m.Packages[0].Tags, len(tags))
			assert.Subset(t, m.Packages[0].Tags, tags)
		},
		"IgnoresExcludedTags": func(t *testing.T, m *Golang) {
			tags := []string{"tag"}
			m.Packages = []GolangPackage{
				{
					Name:        "task",
					Tags:        tags,
					ExcludeTags: defaultTags[:1],
				},
			}
			m.ApplyDefaultTags()
			assert.Len(t, m.Packages[0].Tags, len(tags)+len(defaultTags)-1)
			assert.Subset(t, m.Packages[0].Tags, tags)
			assert.Subset(t, m.Packages[0].Tags, defaultTags[1:])
		},
	} {
		t.Run(testName, func(t *testing.T) {
			m := Golang{
				DefaultTags: defaultTags,
			}
			testCase(t, &m)
		})
	}
}
