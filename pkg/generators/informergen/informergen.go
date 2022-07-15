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

package informergen

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"k8s.io/code-generator/cmd/client-gen/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"

	"github.com/kcp-dev/code-generator/pkg/flag"
	"github.com/kcp-dev/code-generator/pkg/generators/clientgen"
	"github.com/kcp-dev/code-generator/pkg/internal/informergen"
	"github.com/kcp-dev/code-generator/pkg/parser"
	"github.com/kcp-dev/code-generator/pkg/util"
)

const (
	// GeneratorName is the name of the generator.
	GeneratorName = "informer"
	// packageName for typed client wrappers.
	typedPackageName = "externalversions"
)

type Generator struct {
	// inputDir is the path where types are defined.
	inputDir string

	//inputPkgPath stores the input package for the apis.
	inputPkgPath string

	// outputPkgPath stores the output package path for informers.
	outputPkgPath string

	// baseOutputPkgPath stores the base output package path for generated code.
	baseOutputPkgPath string

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
		clientgen.GenclientMarker,
		clientgen.NonNamespacedMarker,
		clientgen.SkipVerbsMarker,
		clientgen.OnlyVerbsMarker,
		clientgen.GroupNameMarker,
	); err != nil {
		return nil, fmt.Errorf("error registering markers")
	}
	return reg, nil
}

func (g Generator) GetName() string {
	return GeneratorName
}

// Run validates the input from the flags and sets default values, after which
// it calls the custom client genrator to create wrappers. If there are any
// errors while generating interface wrappers, it prints it out.
func (g Generator) Run(ctx *genall.GenerationContext, f flag.Flags) error {
	var err error
	if err = g.configure(f); err != nil {
		return err
	}

	if g.groupVersionKinds, err = g.GetGVKs(ctx); err != nil {
		return err
	}

	return g.generate(ctx)
}

// configure sets the Generator's configuration using the given flags.
func (g *Generator) configure(f flag.Flags) error {
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
		g.baseOutputPkgPath = util.GetCleanRealtivePath(pkg, filepath.Clean(g.outputDir))
	} else {
		g.baseOutputPkgPath = pkg
	}
	g.outputPkgPath = filepath.Join(g.baseOutputPkgPath, "informers", typedPackageName)

	g.clientSetAPIPath = f.ClientsetAPIPath

	g.headerText, err = util.GetHeaderText(f.GoHeaderFilePath)
	if err != nil {
		return err
	}

	gvs, err := clientgen.GetGV(f)
	if err != nil {
		return err
	}

	g.groupVersions = append(g.groupVersions, gvs...)

	return nil
}

func (g *Generator) GetGVKs(ctx *genall.GenerationContext) (map[parser.Group]map[types.PackageVersion][]parser.Kind, error) {

	gvks := map[parser.Group]map[types.PackageVersion][]parser.Kind{}

	for _, gv := range g.groupVersions {
		group := parser.Group{Name: gv.Group.String(), GoName: gv.Group.String(), FullName: gv.Group.String()}
		for _, packageVersion := range gv.Versions {

			abs, err := filepath.Abs(g.inputDir)
			if err != nil {
				return nil, err
			}
			path := filepath.Join(abs, group.Name, packageVersion.String())
			pkgs, err := loader.LoadRootsWithConfig(&packages.Config{
				Dir: g.inputDir, Mode: packages.NeedTypesInfo,
			}, path)
			if err != nil {
				return nil, err
			}
			ctx.Roots = pkgs
			for _, root := range ctx.Roots {
				packageMarkers, _ := markers.PackageMarkers(ctx.Collector, root)
				if packageMarkers != nil {
					val, ok := packageMarkers.Get(clientgen.GroupNameMarker.Name).(markers.RawArguments)
					if ok {
						group.FullName = string(val)
						groupGoName := strings.Split(group.FullName, ".")[0]
						if groupGoName != "" {
							group.GoName = groupGoName
						}
					}
				}

				// Initialize the map down here so that we can use the group with the proper GoName as the key
				if _, ok := gvks[group]; !ok {
					gvks[group] = map[types.PackageVersion][]parser.Kind{}
				}
				if _, ok := gvks[group][packageVersion]; !ok {
					gvks[group][packageVersion] = []parser.Kind{}
				}

				if typeErr := markers.EachType(ctx.Collector, root, func(info *markers.TypeInfo) {

					// if not enabled for this type, skip
					if !clientgen.IsEnabledForMethod(info) {
						return
					}
					namespaced := !clientgen.IsClusterScoped(info)
					gvks[group][packageVersion] = append(gvks[group][packageVersion], parser.NewKind(info.Name, namespaced))

				}); typeErr != nil {
					return nil, typeErr
				}
			}
			sort.Slice(gvks[group][packageVersion], func(i, j int) bool {
				return gvks[group][packageVersion][i].String() < gvks[group][packageVersion][j].String()
			})
			if len(gvks[group][packageVersion]) == 0 {
				klog.Warningf("No types discovered for %s:%s, will skip generation for this GroupVersion", group.Name, packageVersion.String())
				delete(gvks[group], packageVersion)
			}
		}
		if len(gvks[group]) == 0 {
			delete(gvks, group)
		}
	}

	return gvks, nil
}

// generate first generates the wrapper for all the interfaces provided in the input.
// Then for each type defined in the input, it recursively wraps the subsequent
// interfaces to be kcp-aware.
func (g *Generator) generate(ctx *genall.GenerationContext) error {
	groups := []parser.Group{}
	for group := range g.groupVersionKinds {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	klog.Info("Generating informer factory")
	if err := g.writeFactory(ctx, groups); err != nil {
		return err
	}

	klog.Info("Generating informer factory interfaces")
	if err := g.writeFactoryInterface(ctx); err != nil {
		return err
	}

	klog.Info("Generating generic informer")
	if err := g.writeGeneric(ctx, groups); err != nil {
		return err
	}

	for group, versionKinds := range g.groupVersionKinds {
		versions := []types.PackageVersion{}
		for version := range versionKinds {
			versions = append(versions, version)
		}
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].Version.String() < versions[j].Version.String()
		})
		klog.Infof("Generating group interface for %s", group.Name)
		if err := g.writeGroupInterface(ctx, group, versions); err != nil {
			klog.Error(err)
			continue
		}

		for version, kinds := range versionKinds {
			klog.Infof("Generating version interface for %s:%s", group.Name, version.String())
			if err := g.writeVersionInterface(ctx, group, version, kinds); err != nil {
				klog.Error(err)
				continue
			}
			for _, kind := range kinds {
				klog.Infof("Generating informer for GVK %s:%s/%s", group.Name, version.String(), kind.String())
				if err := g.writeInformer(ctx, group, version, kind); err != nil {
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

func (g *Generator) writeFactory(ctx *genall.GenerationContext, groups []parser.Group) error {
	var out bytes.Buffer

	if err := g.writeHeader(&out); err != nil {
		return err
	}

	factory := informergen.Factory{
		OutputPackage:    g.outputPkgPath,
		ClientsetPackage: g.clientSetAPIPath,
		Groups:           groups,

		PackageName: "externalversions",
	}
	if err := factory.WriteContent(&out); err != nil {
		return err
	}

	outBytes := out.Bytes()
	formatted, err := format.Source(outBytes)
	if err != nil {
		return err
	}

	return util.WriteContent(formatted, "factory.go", filepath.Join(g.outputDir, "informers", typedPackageName))
}

func (g *Generator) writeFactoryInterface(ctx *genall.GenerationContext) error {
	var out bytes.Buffer

	if err := g.writeHeader(&out); err != nil {
		return err
	}

	factoryInterface := informergen.FactoryInterface{
		ClientsetPackage: g.clientSetAPIPath,
	}
	if err := factoryInterface.WriteContent(&out); err != nil {
		return err
	}

	outBytes := out.Bytes()
	formatted, err := format.Source(outBytes)
	if err != nil {
		return err
	}

	return util.WriteContent(formatted, "factory_interfaces.go", filepath.Join(g.outputDir, "informers", typedPackageName, "internalinterfaces"))
}

func (g *Generator) writeGeneric(ctx *genall.GenerationContext, groups []parser.Group) error {
	var out bytes.Buffer

	if err := g.writeHeader(&out); err != nil {
		return err
	}

	generic := informergen.Generic{
		InputPackage:      g.inputPkgPath,
		PackageName:       typedPackageName,
		GroupVersionKinds: g.groupVersionKinds,
		Groups:            groups,
	}
	if err := generic.WriteContent(&out); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return err
	}

	return util.WriteContent(formatted, "generic.go", filepath.Join(g.outputDir, "informers", typedPackageName))
}

func (g *Generator) writeGroupInterface(ctx *genall.GenerationContext, group parser.Group, versions []types.PackageVersion) error {
	var out bytes.Buffer
	if err := g.writeHeader(&out); err != nil {
		return err
	}
	groupInterface := informergen.GroupInterface{
		OutputPackage: g.outputPkgPath,
		Group:         group,
		Versions:      versions,
	}

	if err := groupInterface.WriteContent(&out); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return err
	}

	return util.WriteContent(formatted, "interface.go", filepath.Join(g.outputDir, "informers", typedPackageName, group.Name))
}

func (g *Generator) writeVersionInterface(ctx *genall.GenerationContext, group parser.Group, version types.PackageVersion, kinds []parser.Kind) error {
	var out bytes.Buffer
	if err := g.writeHeader(&out); err != nil {
		return err
	}

	versionInterface := informergen.VersionInterface{
		OutputPackage: g.outputPkgPath,
		PackageName:   version.String(),
		Kinds:         kinds,
	}

	if err := versionInterface.WriteContent(&out); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return err
	}

	return util.WriteContent(formatted, "interface.go", filepath.Join(g.outputDir, "informers", typedPackageName, group.Name, version.Version.String()))
}

func (g *Generator) writeInformer(ctx *genall.GenerationContext, group parser.Group, version types.PackageVersion, kind parser.Kind) error {
	var out bytes.Buffer
	if err := g.writeHeader(&out); err != nil {
		return err
	}

	informer := informergen.Informer{
		InputPackage:     g.inputPkgPath,
		OutputPackage:    g.outputPkgPath,
		ClientsetPackage: g.clientSetAPIPath,
		ListerPackage:    filepath.Join(g.baseOutputPkgPath, "listers"),
		PackageName:      version.String(),
		Group:            group,
		Version:          version,
		Kind:             kind,
	}

	if err := informer.WriteContent(&out); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return err
	}

	return util.WriteContent(formatted, strings.ToLower(kind.Plural())+".go", filepath.Join(g.outputDir, "informers", typedPackageName, group.Name, version.Version.String()))
}
