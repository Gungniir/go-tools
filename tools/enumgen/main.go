package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	. "github.com/dave/jennifer/jen"
)

type foundVar struct {
	Name    string // идентификатор переменной: TagDynamicClient
	Order   int
	LitName string // значение поля name: "dynamic_client"
}

func main() {
	var outFile string
	var marker string
	var onlyType string // если указан, игнорируем маркер и генерим только для этого типа
	var filePath string // если указан, обрабатываем только один файл

	flag.StringVar(&outFile, "out", "bitsets_gen.go", "output file")
	flag.StringVar(&marker, "marker", "bitset:gen", "doc comment marker on type to opt-in")
	flag.StringVar(&onlyType, "type", "", "explicit type name (bypass marker)")
	flag.StringVar(&filePath, "file", "", "single file mode: process only this file")
	flag.Parse()

	// Подготовим структуры для AST
	fset := token.NewFileSet()

	typeVars := map[string][]foundVar{} // typeName -> vars
	targetTypes := map[string]bool{}    // интересующие типы (по маркеру или -type)
	pkgName := ""
	order := 0

	if filePath != "" {
		// ----- РЕЖИМ ОДНОГО ФАЙЛА -----
		abs, err := filepath.Abs(filePath)
		if err != nil {
			log.Fatal(err)
		}
		file, err := parser.ParseFile(fset, abs, nil, parser.ParseComments)
		if err != nil {
			log.Fatal(err)
		}
		pkgName = file.Name.Name

		// 1) Определяем интересующие типы
		if onlyType != "" {
			targetTypes[onlyType] = true
		} else {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if hasMarker(ts.Doc, marker) || hasMarker(gd.Doc, marker) {
						targetTypes[ts.Name.Name] = true
					}
				}
			}
			if len(targetTypes) == 0 {
				log.Fatalf("no annotated types found in %s (marker %q). Pass -type TypeName to override.", filePath, marker)
			}
		}

		// 2) Собираем переменные только из этого файла
		ast.Inspect(file, func(n ast.Node) bool {
			gd, ok := n.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				return true
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if name == nil || len(vs.Values) == 0 || i >= len(vs.Values) {
						continue
					}
					cl, ok := vs.Values[i].(*ast.CompositeLit)
					if !ok {
						continue
					}
					typeName := compositeTypeName(cl.Type)
					if typeName == "" || !targetTypes[typeName] {
						continue
					}
					litName, okName := extractNameFromCompositeLit(cl)
					if !okName || litName == "" {
						log.Fatalf("variable %s: cannot extract Tag.name string literal", name.Name)
					}
					if hasSpaceRune(litName) {
						log.Fatalf("variable %s: name %q contains whitespace", name.Name, litName)
					}

					typeVars[typeName] = append(typeVars[typeName], foundVar{
						Name:    name.Name,
						Order:   order,
						LitName: litName,
					})
					order++
				}
			}
			return true
		})

	} else {
		// ----- ПАКЕТНЫЙ РЕЖИМ -----
		pkgDir, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		pkgs, err := parser.ParseDir(fset, pkgDir, func(fi os.FileInfo) bool {
			n := fi.Name()
			if strings.HasSuffix(n, "_gen.go") || strings.HasSuffix(n, ".pb.go") || strings.HasSuffix(n, "_string.go") {
				return false
			}
			if strings.HasSuffix(n, "_test.go") {
				return false
			}
			return strings.HasSuffix(n, ".go")
		}, parser.ParseComments)
		if err != nil {
			log.Fatal(err)
		}
		if len(pkgs) == 0 {
			log.Fatal("no packages found")
		}

		var pkg *ast.Package
		for _, p := range pkgs {
			pkg = p
			break
		}
		for _, f := range pkg.Files {
			pkgName = f.Name.Name
			break
		}

		// 1) Интересующие типы
		if onlyType != "" {
			targetTypes[onlyType] = true
		} else {
			for _, f := range pkg.Files {
				for _, decl := range f.Decls {
					gd, ok := decl.(*ast.GenDecl)
					if !ok || gd.Tok != token.TYPE {
						continue
					}
					for _, spec := range gd.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						if hasMarker(ts.Doc, marker) || hasMarker(gd.Doc, marker) {
							targetTypes[ts.Name.Name] = true
						}
					}
				}
			}
			if len(targetTypes) == 0 {
				log.Fatalf("no annotated types found in package (marker %q). Pass -type TypeName to override.", marker)
			}
		}

		// 2) Переменные (из всех файлов пакета)
		for _, f := range pkg.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				gd, ok := n.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					return true
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range vs.Names {
						if name == nil || len(vs.Values) == 0 || i >= len(vs.Values) {
							continue
						}
						cl, ok := vs.Values[i].(*ast.CompositeLit)
						if !ok {
							continue
						}
						typeName := compositeTypeName(cl.Type)
						if typeName == "" || !targetTypes[typeName] {
							continue
						}
						litName, okName := extractNameFromCompositeLit(cl)
						if !okName || litName == "" {
							log.Fatalf("variable %s: cannot extract Tag.name string literal", name.Name)
						}
						if hasSpaceRune(litName) {
							log.Fatalf("variable %s: name %q contains whitespace", name.Name, litName)
						}

						typeVars[typeName] = append(typeVars[typeName], foundVar{
							Name:    name.Name,
							Order:   order,
							LitName: litName,
						})
						order++
					}
				}
				return true
			})
		}
	}

	// Сортировка по порядку встречи
	for t := range typeVars {
		sort.SliceStable(typeVars[t], func(i, j int) bool { return typeVars[t][i].Order < typeVars[t][j].Order })
	}

	// Генерация
	if pkgName == "" {
		log.Fatal("cannot determine package name")
	}
	f := NewFile(pkgName)
	f.HeaderComment("Code generated by collgen; DO NOT EDIT.")

	for typeName, vars := range typeVars {
		if len(vars) == 0 {
			continue
		}
		genOne(f, typeName, vars)
	}

	if len(typeVars) == 0 {
		log.Fatalf("no vars for selected types found")
	}

	tmp := outFile + ".tmp"
	if err := f.Save(tmp); err != nil {
		log.Fatal(err)
	}
	if err := os.Rename(tmp, outFile); err != nil {
		_ = os.Remove(tmp)
		log.Fatal(err)
	}
	fmt.Printf("generated %s for %d type(s)\n", outFile, len(typeVars))
}

func hasMarker(doc *ast.CommentGroup, marker string) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		if strings.Contains(c.Text, marker) {
			return true
		}
	}
	return false
}

func compositeTypeName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if x.Sel != nil {
			return x.Sel.Name
		}
	}
	return ""
}

// возвращает строку из поля name, если найдена
func extractNameFromCompositeLit(cl *ast.CompositeLit) (string, bool) {
	// 1) позиционный: Tag{"xxx"}
	if len(cl.Elts) > 0 {
		if bl, ok := cl.Elts[0].(*ast.BasicLit); ok && bl.Kind == token.STRING {
			if s, err := strconv.Unquote(bl.Value); err == nil {
				return s, true
			}
		}
	}
	// 2) именованный: Tag{name: "xxx"}
	for _, e := range cl.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "name" {
				if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
					if s, err := strconv.Unquote(bl.Value); err == nil {
						return s, true
					}
				}
			}
		}
	}
	return "", false
}

func hasSpaceRune(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func getConstName(typeName string, vName string) string {
	return fmt.Sprintf("_%s_%s_Name", typeName, vName)
}

func genOne(f *File, typeName string, vars []foundVar) {
	// формируем уникальные идентификаторы: _<TypeName>_<VarName>_Name
	f.Const().DefsFunc(func(g *Group) {
		for _, v := range vars {
			g.Id(getConstName(typeName, v.Name)).Op("=").Lit(v.LitName)
		}
	})

	// --- String() string
	f.Func().Params(Id("c").Id(typeName)).Id("String").Params().String().
		BlockFunc(func(b *Group) {
			b.Switch(Id("c")).BlockFunc(func(s *Group) {
				for _, v := range vars {
					s.Case(Id(v.Name)).Block(
						Return(Id(getConstName(typeName, v.Name))),
					)
				}
			})
			b.Return(Lit(""))
		})

	// --- Parse<Coll>(s string) (switch по константам)
	f.Func().Id("Parse"+typeName).
		Params(Id("s").String()).
		Params(Id(typeName), Error()).
		Block(
			Id("c").Op(":=").Id(typeName).Values(),
			Switch(Id("s")).BlockFunc(func(s *Group) {
				for _, v := range vars {
					s.Case(Id(getConstName(typeName, v.Name))).Block(
						Return(Id(v.Name), Nil()),
					)
				}
				s.Default().Block(
					Return(Id("c"), Qual("fmt", "Errorf").Call(Lit("unknown %s: %q"), Lit(typeName), Id("s"))),
				)
			}),
			Return(Id("c"), Nil()),
		)

	// --- JSON: MarshalJSON/UnmarshalJSON
	// MarshalJSON кодирует enum как JSON-строку
	f.Func().
		Params(Id("c").Id(typeName)).
		Id("MarshalJSON").
		Params().
		Params(Index().Byte(), Error()).
		Block(
			Return(
				Qual("encoding/json", "Marshal").
					Call(Id("c").Dot("String").Call()),
			),
		)

	// UnmarshalJSON ожидает JSON-строку
	f.Func().
		Params(Id("c").Op("*").Id(typeName)).
		Id("UnmarshalJSON").
		Params(Id("b").Index().Byte()).
		Error().
		Block(
			Var().Id("s").String(),
			If(Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(Id("b"), Op("&").Id("s")), Err().Op("!=").Nil()).
				Block(Return(Err())),
			List(Id("v"), Err()).Op(":=").Id("Parse"+typeName).Call(Id("s")),
			If(Err().Op("!=").Nil()).Block(Return(Err())),
			Op("*").Id("c").Op("=").Id("v"),
			Return(Nil()),
		)
}
