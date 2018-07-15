// +build generate

// All of the generate commands.
// These are all added to the project, so the `-run` option should be used, otherwise
// you'll spend a long time waiting for a dev build of the project

//go:generate -command static esc -o static.go -prefix site/adminator/build/ -pkg main site/adminator/build/
//go:generate -command static-echo echo "generated static.go..."
//go:generate -command dev_win env GOOS=windows GOARCH=amd64 go build -o ./bin/windows_amd64/check.exe -tags dev
//go:generate -command dev_mos env GOOS=darwin GOARCH=amd64  go build -o ./bin/macos_amd64/check -tags dev
//go:generate -command dev_lin env GOOS=linux GOARCH=amd64   go build -o ./bin/linux_amd64/check -tags dev
//go:generate -command prd_win env GOOS=windows GOARCH=amd64 go build -o ./bin/windows_amd64/check.exe
//go:generate -command prd_mos env GOOS=darwin GOARCH=amd64  go build -o ./bin/macos_amd64/check
//go:generate -command prd_lin env GOOS=linux GOARCH=amd64   go build -o ./bin/linux_amd64/check

// The exec of the commands (order matters here):

//go:generate static
//go:generate static-echo

//go:generate -command dev_win-prd_win-dev_mos-prd_mos-dev_lin-prd_lin go run generate.go
//go:generate dev_win-prd_win-dev_mos-prd_mos-dev_lin-prd_lin go

//go:generate prd_win
//go:generate prd_mos
//go:generate prd_lin
//go:generate dev_win
//go:generate dev_mos
//go:generate dev_lin

package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {

	f, err := os.OpenFile("build.go", os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", b, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	ast.Inspect(node, func(n ast.Node) bool {
		val, ok := n.(*ast.ValueSpec)
		if ok {
			for _, id := range val.Names {
				switch id.Name {
				case "verBuild":
					for _, v := range val.Values {
						i, err := strconv.Atoi(strings.Trim(v.(*ast.BasicLit).Value, `\"`))
						if err != nil {
							log.Fatal(err)
						}
						v.(*ast.BasicLit).Value = fmt.Sprintf(`"%d"`, i+1)
					}
				case "verDate":
					for _, v := range val.Values {
						v.(*ast.BasicLit).Value = fmt.Sprintf(`"%s"`, time.Now().Format(time.RFC1123))
					}
				}
			}
		}
		return true
	})

	f.Truncate(0)
	f.Seek(0,0)
	format.Node(f, fset, node)
}

// To add HTML template pages to the document use:
// go generate -run "static" generate.go

// To generate a dev build:
// go generate "dev_lin" generate.go

// To generate a production build
// go generate "prd_lin" generate.go
