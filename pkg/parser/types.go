package parser

import (
	"github.com/kcp-dev/code-generator/pkg/util"
	"github.com/kcp-dev/code-generator/third_party/namer"
)

type Kind struct {
	kind       string
	namespaced bool
	namer      namer.Namer
}

type Group struct {
	Name     string
	GoName   string
	FullName string
}

func (k *Kind) Plural() string {
	return k.namer.Name(k.kind)
}

func (k *Kind) String() string {
	return k.kind
}

func (k *Kind) IsNamespaced() bool {
	return k.namespaced
}

func NewKind(kind string, namespaced bool) Kind {
	return Kind{
		kind:       kind,
		namespaced: namespaced,
		namer: namer.Namer{
			Finalize: util.UpperFirst,
			Exceptions: map[string]string{
				"Endpoints": "Endpoints",
			},
		},
	}
}
