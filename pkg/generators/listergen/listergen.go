/*
Copyright 2022 The KCP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package listergen

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io"
	"path/filepath"
	"strings"

	"github.com/kcp-dev/code-generator/pkg/flag"
	"github.com/kcp-dev/code-generator/pkg/internal/listergen"
	"github.com/kcp-dev/code-generator/pkg/parser"
	"github.com/kcp-dev/code-generator/pkg/util"
	"golang.org/x/tools/go/packages"
	"k8s.io/code-generator/cmd/client-gen/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"
)

const (
	// GeneratorName is the name of the generator.
	GeneratorName = "lister"
)

type Generator struct {
	// inputDir is the path where types are defined.
	inputDir string

	//inputPkgPath stores the input package for the apis.
	inputPkgPath string

	// outputPkgPath stores the output package path for informers.
	outputPkgPath string

	// output Dir where the wrappers are to be written.
	outputDir string

	// GroupVersions for whom the clients are to be generated.
	groupVersions []types.GroupVersions

	// GroupVersionKinds contains all the needed APIs to scaffold
	groupVersionKinds map[parser.Group]map[types.PackageVersion][]parser.Kind

	// headerText is the header text to be added to generated wrappers.
	// It is obtained from `--go-header-text` flag.
	headerText string

	// path to where generated clientsets are found.
	clientSetAPIPath string
}

func (g Generator) RegisterMarker() (*markers.Registry, error) {
	reg := &markers.Registry{}
	if err := markers.RegisterAll(reg,
		parser.GenclientMarker,
		parser.NonNamespacedMarker,
		parser.SkipVerbsMarker,
		parser.OnlyVerbsMarker,
		parser.GroupNameMarker,
	); err != nil {
		return nil, fmt.Errorf("error registering markers")
	}
	return reg, nil
}

func (g Generator) GetName() string {
	return GeneratorName
}

func (g Generator) Run(ctx *genall.GenerationContext, f flag.Flags) error {
	var err error

	if err = flag.ValidateFlags(f); err != nil {
		return err
	}
	if err = g.setDefaults(f); err != nil {
		return err
	}

	if g.groupVersionKinds, err = parser.GetGVKs(ctx, g.inputDir, g.groupVersions); err != nil {
		return err
	}

	if err = g.generate(ctx); err != nil {
		return err
	}

	// print all the errors consolidated from packages in the generation context.
	// skip the type errors since they occur when input path does not contain
	// go.mod files.
	hadErr := loader.PrintErrors(ctx.Roots, packages.TypeError)
	if hadErr {
		return fmt.Errorf("generator did not run successfully")
	}
	return nil
}

func (g *Generator) setDefaults(f flag.Flags) (err error) {
	if err := flag.ValidateFlags(f); err != nil {
		return err
	}

	g.inputDir = f.InputDir
	absoluteInputDir, err := filepath.Abs(g.inputDir)
	if err != nil {
		return err
	}

	pkg, hasGoMod := util.CurrentPackage(absoluteInputDir)
	if len(pkg) == 0 {
		return fmt.Errorf("error finding the module path for this package %q", f.InputDir)
	}
	cleanPkgPath := util.CleanInputDir(g.inputDir)
	if !hasGoMod && cleanPkgPath != "" {
		g.inputPkgPath = filepath.Join(pkg, cleanPkgPath)
	} else {
		g.inputPkgPath = pkg
	}
	g.outputDir = f.OutputDir
	pkg, hasGoMod = util.CurrentPackage(f.OutputDir)
	if len(pkg) == 0 {
		return fmt.Errorf("error finding the module path for this package %q", f.OutputDir)
	}

	if !hasGoMod {
		g.outputPkgPath = util.GetCleanRealtivePath(pkg, filepath.Clean(g.outputDir))
	} else {
		g.outputPkgPath = pkg
	}

	g.clientSetAPIPath = f.ClientsetAPIPath

	g.headerText, err = util.GetHeaderText(f.GoHeaderFilePath)
	if err != nil {
		return err
	}

	gvs, err := parser.GetGV(f)
	if err != nil {
		return err
	}

	g.groupVersions = append(g.groupVersions, gvs...)

	return nil
}

func (g *Generator) generate(ctx *genall.GenerationContext) error {
	for group, versionKinds := range g.groupVersionKinds {
		for version, kinds := range versionKinds {
			for _, kind := range kinds {
				var out bytes.Buffer
				if err := g.writeHeader(&out); err != nil {
					klog.Error(err)
					continue
				}
				klog.Infof("Generating lister for GroupVersionKind %s:%s/%s", group.Name, version.String(), kind.String())
				lister := listergen.Lister{
					Group:   group,
					Version: version,
					Kind:    kind,
					APIPath: filepath.Join(g.inputPkgPath, group.Name, version.String()),
				}
				if err := lister.WriteContent(&out); err != nil {
					klog.Error(err)
					continue
				}

				outBytes := out.Bytes()
				formattedBytes, err := format.Source(outBytes)
				if err != nil {
					klog.Error(err)
					continue
				}
				filename := strings.ToLower(kind.String()) + util.ExtensionGo
				err = util.WriteContent(formattedBytes, filename, filepath.Join(g.outputDir, "listers", group.Name, string(version.Version)))
				if err != nil {
					klog.Error(err)
					continue
				}
			}
		}
	}

	return nil
}

func (g *Generator) writeHeader(out io.Writer) error {
	n, err := out.Write([]byte(g.headerText))
	if err != nil {
		return err
	}

	if n < len([]byte(g.headerText)) {
		return errors.New("header text was not written properly.")
	}
	return nil
}
