package parser

import (
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"k8s.io/code-generator/cmd/client-gen/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"

	"github.com/kcp-dev/code-generator/pkg/generators/clientgen"
)

func GetGVKs(ctx *genall.GenerationContext, inputDir string, groupVersions []types.GroupVersions) (map[Group]map[types.PackageVersion][]Kind, error) {

	gvks := map[Group]map[types.PackageVersion][]Kind{}

	for _, gv := range groupVersions {
		group := Group{Name: gv.Group.String(), GoName: gv.Group.String(), FullName: gv.Group.String()}
		for _, packageVersion := range gv.Versions {

			abs, err := filepath.Abs(inputDir)
			if err != nil {
				return nil, err
			}
			path := filepath.Join(abs, group.Name, packageVersion.String())
			pkgs, err := loader.LoadRootsWithConfig(&packages.Config{
				Dir: inputDir, Mode: packages.NeedTypesInfo,
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
					gvks[group] = map[types.PackageVersion][]Kind{}
				}
				if _, ok := gvks[group][packageVersion]; !ok {
					gvks[group][packageVersion] = []Kind{}
				}

				if typeErr := markers.EachType(ctx.Collector, root, func(info *markers.TypeInfo) {

					// if not enabled for this type, skip
					if !clientgen.IsEnabledForMethod(info) {
						return
					}
					namespaced := !clientgen.IsClusterScoped(info)
					gvks[group][packageVersion] = append(gvks[group][packageVersion], NewKind(info.Name, namespaced))

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
