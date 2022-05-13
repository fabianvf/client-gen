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

package example

import (
	informers "github.com/kcp-dev/code-generator/examples/pkg/generated/informers/example"

	informersv1 "github.com/kcp-dev/code-generator/examples/pkg/generated/informers/example/v1"
)

type Interface interface {
	V1() informersv1.Interface
}

type group struct {
	delegate informers.Interface
}

func New(delegate informers.Interface) Interface {
	return &group{delegate: delegate}
}
