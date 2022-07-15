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
	"sort"
	"strings"
	"text/template"

	"github.com/kcp-dev/code-generator/pkg/parser"
	"github.com/kcp-dev/code-generator/pkg/util"
	"k8s.io/code-generator/cmd/client-gen/types"
)

var (
	templateFuncs = template.FuncMap{
		"upperFirst":   util.UpperFirst,
		"lowerFirst":   util.LowerFirst,
		"toLower":      strings.ToLower,
		"sortVersions": sortVersions,
	}
)

func sortVersions(versionKinds map[types.PackageVersion][]parser.Kind) []types.PackageVersion {
	versions := []types.PackageVersion{}
	for version := range versionKinds {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version.String() < versions[j].Version.String()
	})
	return versions
}
