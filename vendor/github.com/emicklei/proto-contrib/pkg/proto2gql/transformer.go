package proto2gql

import (
	"io"
	"net/http"
	"strings"

	"github.com/emicklei/proto"
)

type (
	Filter = func(typeName string) bool

	ExternalPackage struct {
		url      string
		resolved bool
	}

	Transformer struct {
		out        io.Writer
		filename   string
		imports    map[string]*ExternalPackage
		pkgAliases map[string]string
		noPrefix   bool
		filter     Filter
	}
)

func bypassFilter(_ string) bool {
	return true
}

func NewTransformer(out io.Writer, opts ...func(transformer *Transformer)) *Transformer {
	res := &Transformer{
		out,
		"",
		make(map[string]*ExternalPackage),
		make(map[string]string),
		false,
		bypassFilter,
	}

	for _, opt := range opts {
		opt(res)
	}

	return res
}

func (t *Transformer) DisablePrefix(value bool) {
	t.noPrefix = value
}

func (t *Transformer) Import(name string, url string) {
	name = strings.TrimSpace(name)

	_, exists := t.imports[name]

	if exists == false {
		t.imports[name] = &ExternalPackage{strings.TrimSpace(url), false}
	}
}

func (t *Transformer) SetPackageAlias(pkg, alias string) {
	t.pkgAliases[pkg] = alias
}

func (t *Transformer) SetFilename(filename string) {
	t.filename = filename
}

func (t *Transformer) SetFilter(filter Filter) {
	if filter != nil {
		t.filter = filter
	}
}

func (t *Transformer) Transform(input io.Reader) error {
	parser := proto.NewParser(input)
	parser.Filename(t.filename)

	def, err := parser.Parse()

	if err != nil {
		return err
	}

	visitor := NewVisitor(&Converter{
		noPrefix:   t.noPrefix,
		pkgAliases: t.pkgAliases,
	}, t.filter)

	toDownload := make(map[string]*ExternalPackage)

	for _, element := range def.Elements {

		switch element := element.(type) {
		case *proto.Import:
			// we collect imports to be resolved afterwards
			pkg, needed := t.imports[element.Filename]

			if needed == true {
				if pkg.resolved == false {
					_, added := toDownload[pkg.url]

					if added == false {
						toDownload[pkg.url] = pkg
					}
				}
			}
		}

		element.Accept(visitor)

		visitor.Flush(t.out)
	}

	if len(toDownload) > 0 {
		packages := make([]*ExternalPackage, 0, len(toDownload))

		for _, pkg := range toDownload {
			packages = append(packages, pkg)
		}

		return t.resolveExternalPackages(packages)
	}

	return nil
}

func (t *Transformer) resolveExternalPackages(packages []*ExternalPackage) error {
	for _, pkg := range packages {
		resp, err := http.Get(pkg.url)

		if err != nil {
			return err
		}

		pkg.resolved = true

		err = t.Transform(resp.Body)

		if err != nil {
			return err
		}
	}

	return nil
}
