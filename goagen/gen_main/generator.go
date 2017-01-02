package genmain

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goadesign/goa/design"
	"github.com/goadesign/goa/goagen/codegen"
	"github.com/goadesign/goa/goagen/utils"
)

//NewGenerator returns an initialized instance of a JavaScript Client Generator
func NewGenerator(options ...Option) *Generator {
	g := &Generator{}

	for _, option := range options {
		option(g)
	}

	return g
}

// Generator is the application code generator.
type Generator struct {
	API       *design.APIDefinition // The API definition
	OutDir    string                // Path to output directory
	DesignPkg string                // Path to design package, only used to mark generated files.
	Target    string                // Name of generated "app" package
	Force     bool                  // Whether to override existing files
	genfiles  []string              // Generated files
}

// Generate is the generator entry point called by the meta generator.
func Generate() (files []string, err error) {
	var (
		outDir, designPkg, target, ver string
		force                          bool
	)

	set := flag.NewFlagSet("main", flag.PanicOnError)
	set.StringVar(&outDir, "out", "", "")
	set.StringVar(&designPkg, "design", "", "")
	set.StringVar(&target, "pkg", "app", "")
	set.StringVar(&ver, "version", "", "")
	set.BoolVar(&force, "force", false, "")
	set.Bool("notest", false, "")
	set.Parse(os.Args[1:])

	if err := codegen.CheckVersion(ver); err != nil {
		return nil, err
	}

	target = codegen.Goify(target, false)
	g := &Generator{OutDir: outDir, DesignPkg: designPkg, Target: target, Force: force, API: design.Design}

	return g.Generate()
}

// Generate produces the skeleton main.
func (g *Generator) Generate() (_ []string, err error) {
	go utils.Catch(nil, func() { g.Cleanup() })

	defer func() {
		if err != nil {
			g.Cleanup()
		}
	}()

	if g.Target == "" {
		g.Target = "app"
	}

	codegen.Reserved[g.Target] = true

	mainFile := filepath.Join(g.OutDir, "main.go")
	if g.Force {
		os.Remove(mainFile)
	}
	funcs := template.FuncMap{
		"tempvar":   tempvar,
		"okResp":    g.okResp,
		"targetPkg": func() string { return g.Target },
	}
	imp, err := codegen.PackagePath(g.OutDir)
	if err != nil {
		return nil, err
	}
	imp = path.Join(filepath.ToSlash(imp), "app")
	_, err = os.Stat(mainFile)
	if err != nil {
		if err = g.createMainFile(mainFile, funcs); err != nil {
			return nil, err
		}
	}
	imports := []*codegen.ImportSpec{
		codegen.SimpleImport("io"),
		codegen.SimpleImport("github.com/goadesign/goa"),
		codegen.SimpleImport(imp),
		codegen.SimpleImport("golang.org/x/net/websocket"),
	}
	err = g.API.IterateResources(func(r *design.ResourceDefinition) error {
		filename := filepath.Join(g.OutDir, codegen.SnakeCase(r.Name)+".go")
		if g.Force {
			os.Remove(filename)
		}
		if _, e := os.Stat(filename); e != nil {
			g.genfiles = append(g.genfiles, filename)
			file, err2 := codegen.SourceFileFor(filename)
			if err2 != nil {
				return err
			}
			file.WriteHeader("", "main", imports)
			if err2 = file.ExecuteTemplate("controller", ctrlT, funcs, r); err2 != nil {
				return err
			}
			err2 = r.IterateActions(func(a *design.ActionDefinition) error {
				if a.WebSocket() {
					return file.ExecuteTemplate("actionWS", actionWST, funcs, a)
				}
				return file.ExecuteTemplate("action", actionT, funcs, a)
			})
			if err2 != nil {
				return err
			}
			if err2 = file.FormatCode(); err2 != nil {
				return err2
			}
		}
		return nil
	})
	if err != nil {
		return
	}

	return g.genfiles, nil
}

// Cleanup removes all the files generated by this generator during the last invokation of Generate.
func (g *Generator) Cleanup() {
	for _, f := range g.genfiles {
		os.Remove(f)
	}
	g.genfiles = nil
}

// tempCount is the counter used to create unique temporary variable names.
var tempCount int

// tempvar generates a unique temp var name.
func tempvar() string {
	tempCount++
	if tempCount == 1 {
		return "c"
	}
	return fmt.Sprintf("c%d", tempCount)
}

func (g *Generator) createMainFile(mainFile string, funcs template.FuncMap) error {
	g.genfiles = append(g.genfiles, mainFile)
	file, err := codegen.SourceFileFor(mainFile)
	if err != nil {
		return err
	}
	funcs["getPort"] = func(hostport string) string {
		_, port, err := net.SplitHostPort(hostport)
		if err != nil {
			return "8080"
		}
		return port
	}
	outPkg, err := codegen.PackagePath(g.OutDir)
	if err != nil {
		return err
	}
	appPkg := path.Join(outPkg, "app")
	imports := []*codegen.ImportSpec{
		codegen.SimpleImport("time"),
		codegen.SimpleImport("github.com/goadesign/goa"),
		codegen.SimpleImport("github.com/goadesign/goa/middleware"),
		codegen.SimpleImport(appPkg),
	}
	file.Write([]byte("//go:generate goagen bootstrap -d " + g.DesignPkg + "\n\n"))
	file.WriteHeader("", "main", imports)
	data := map[string]interface{}{
		"Name": g.API.Name,
		"API":  g.API,
	}
	if err = file.ExecuteTemplate("main", mainT, funcs, data); err != nil {
		return err
	}
	return file.FormatCode()
}

func (g *Generator) okResp(a *design.ActionDefinition) map[string]interface{} {
	var ok *design.ResponseDefinition
	for _, resp := range a.Responses {
		if resp.Status == 200 {
			ok = resp
			break
		}
	}
	if ok == nil {
		return nil
	}
	var mt *design.MediaTypeDefinition
	var ok2 bool
	if mt, ok2 = design.Design.MediaTypes[design.CanonicalIdentifier(ok.MediaType)]; !ok2 {
		return nil
	}
	view := ok.ViewName
	if view == "" {
		view = design.DefaultView
	}
	pmt, _, err := mt.Project(view)
	if err != nil {
		return nil
	}
	var typeref string
	if pmt.IsError() {
		typeref = `goa.ErrInternal("not implemented")`
	} else {
		name := codegen.GoTypeRef(pmt, pmt.AllRequired(), 1, false)
		var pointer string
		if strings.HasPrefix(name, "*") {
			name = name[1:]
			pointer = "*"
		}
		typeref = fmt.Sprintf("%s%s.%s", pointer, g.Target, name)
		if strings.HasPrefix(typeref, "*") {
			typeref = "&" + typeref[1:]
		}
		typeref += "{}"
	}
	var nameSuffix string
	if view != "default" {
		nameSuffix = codegen.Goify(view, true)
	}
	return map[string]interface{}{
		"Name":    ok.Name + nameSuffix,
		"GoType":  codegen.GoNativeType(pmt),
		"TypeRef": typeref,
	}
}

const mainT = `
func main() {
	// Create service
	service := goa.New({{ printf "%q" .Name }})

	// Mount middleware
	service.Use(middleware.RequestID())
	service.Use(middleware.LogRequest(true))
	service.Use(middleware.ErrorHandler(service, true))
	service.Use(middleware.Recover())
{{ $api := .API }}
{{ range $name, $res := $api.Resources }}{{ $name := goify $res.Name true }} // Mount "{{$res.Name}}" controller
	{{ $tmp := tempvar }}{{ $tmp }} := New{{ $name }}Controller(service)
	{{ targetPkg }}.Mount{{ $name }}Controller(service, {{ $tmp }})
{{ end }}

	// Start service
	if err := service.ListenAndServe(":{{ getPort .API.Host }}"); err != nil {
		service.LogError("startup", "err", err)
	}
}
`

const ctrlT = `// {{ $ctrlName := printf "%s%s" (goify .Name true) "Controller" }}{{ $ctrlName }} implements the {{ .Name }} resource.
type {{ $ctrlName }} struct {
	*goa.Controller
}

// New{{ $ctrlName }} creates a {{ .Name }} controller.
func New{{ $ctrlName }}(service *goa.Service) *{{ $ctrlName }} {
	return &{{ $ctrlName }}{Controller: service.NewController("{{ $ctrlName }}")}
}
`

const actionT = `{{ $ctrlName := printf "%s%s" (goify .Parent.Name true) "Controller" }}// {{ goify .Name true }} runs the {{ .Name }} action.
func (c *{{ $ctrlName }}) {{ goify .Name true }}(ctx *{{ targetPkg }}.{{ goify .Name true }}{{ goify .Parent.Name true }}Context) error {
	// {{ $ctrlName }}_{{ goify .Name true }}: start_implement

	// Put your logic here

	// {{ $ctrlName }}_{{ goify .Name true }}: end_implement
{{ $ok := okResp . }}{{ if $ok }} res := {{ $ok.TypeRef }}
{{ end }} return {{ if $ok }}ctx.{{ $ok.Name }}(res){{ else }}nil{{ end }}
}
`

const actionWST = `{{ $ctrlName := printf "%s%s" (goify .Parent.Name true) "Controller" }}// {{ goify .Name true }} runs the {{ .Name }} action.
func (c *{{ $ctrlName }}) {{ goify .Name true }}(ctx *{{ targetPkg }}.{{ goify .Name true }}{{ goify .Parent.Name true }}Context) error {
	c.{{ goify .Name true }}WSHandler(ctx).ServeHTTP(ctx.ResponseWriter, ctx.Request)
	return nil
}

// {{ goify .Name true }}WSHandler establishes a websocket connection to run the {{ .Name }} action.
func (c *{{ $ctrlName }}) {{ goify .Name true }}WSHandler(ctx *{{ targetPkg }}.{{ goify .Name true }}{{ goify .Parent.Name true }}Context) websocket.Handler {
	return func(ws *websocket.Conn) {
		// {{ $ctrlName }}_{{ goify .Name true }}: start_implement

		// Put your logic here

		// {{ $ctrlName }}_{{ goify .Name true }}: end_implement
		ws.Write([]byte("{{ .Name }} {{ .Parent.Name }}"))
		// Dummy echo websocket server
		io.Copy(ws, ws)
	}
}
`
