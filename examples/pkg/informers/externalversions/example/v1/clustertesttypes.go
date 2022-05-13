//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The KCP Authors.

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

// Code generated by kcp code-generator. DO NOT EDIT.

package v1

import (
	informers "github.com/kcp-dev/code-generator/examples/pkg/generated/informers/example/v1"
	listers "github.com/kcp-dev/code-generator/examples/pkg/listers/example/v1"
	"github.com/kcp-dev/kubernetes/src/k8s.io/client-go/tools/cache"
)

type ClusterTestTypeInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() listers.ClusterTestTypeLister
}

type clusterTestTypeInformer struct {
	delegate informers.ClusterTestTypeInformer
}

func (r *clusterTestTypeInformer) Informer() cache.SharedIndexInformer {
	return r.delegate.Informer()
}

func (r *clusterTestTypeInformer) Lister() listers.ClusterTestTypeLister {
	return listers.NewClusterTestTypeLister(r.delegate.Informer().GetIndexer())
}
