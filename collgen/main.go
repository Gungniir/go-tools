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

func genOne(f *File, typeName string, vars []foundVar) {
	type bit struct{ word, bit int }
	coll := typeName + "sCollection"

	count := len(vars)
	words := (count + 63) / 64
	if words == 0 {
		words = 1 // чтобы тип не был пустым
	}

	// предрасчёт битов
	bits := make([]bit, count)
	for i := 0; i < count; i++ {
		bits[i] = bit{word: i / 64, bit: i % 64}
	}

	// --- const-ы с именами (вместо слайса)
	// формируем уникальные идентификаторы: _<TypeName>_<VarName>_Name
	f.Const().DefsFunc(func(g *Group) {
		for _, v := range vars {
			g.Id(fmt.Sprintf("_collgen_%s_%s_Name", typeName, v.Name)).Op("=").Lit(v.LitName)
		}
	})

	// --- struct { mask0, mask1, ... }
	f.Type().Id(coll).StructFunc(func(s *Group) {
		for w := 0; w < words; w++ {
			s.Id(fmt.Sprintf("mask%d", w)).Uint64()
		}
	})

	// --- Add
	f.Func().Params(Id("c").Id(coll)).Id("Add").Params(Id("v").Id(typeName)).Id(coll).
		Block(
			Switch(Id("v")).BlockFunc(func(sb *Group) {
				for i, v := range vars {
					sb.Case(Id(v.Name)).Block(
						Id("c").Dot(fmt.Sprintf("mask%d", bits[i].word)).
							Op("|=").Parens(Lit(1).Op("<<").Lit(bits[i].bit)),
					)
				}
			}),
			Return(Id("c")),
		)

	// --- Remove
	f.Func().Params(Id("c").Id(coll)).Id("Remove").Params(Id("v").Id(typeName)).Id(coll).
		Block(
			Switch(Id("v")).BlockFunc(func(sb *Group) {
				for i, v := range vars {
					sb.Case(Id(v.Name)).Block(
						Id("c").Dot(fmt.Sprintf("mask%d", bits[i].word)).
							Op("&^=").Parens(Lit(1).Op("<<").Lit(bits[i].bit)),
					)
				}
			}),
			Return(Id("c")),
		)

	// --- Has
	f.Func().Params(Id("c").Id(coll)).Id("Has").Params(Id("v").Id(typeName)).Bool().
		Block(
			Switch(Id("v")).BlockFunc(func(sb *Group) {
				for i, v := range vars {
					sb.Case(Id(v.Name)).Block(
						Return(
							Parens(
								Id("c").Dot(fmt.Sprintf("mask%d", bits[i].word)).
									Op("&").Parens(Lit(1).Op("<<").Lit(bits[i].bit)),
							).Op("!=").Lit(0),
						),
					)
				}
			}),
			Return(False()),
		)

	// --- IsEmpty
	f.Func().Params(Id("c").Id(coll)).Id("IsEmpty").Params().Bool().
		BlockFunc(func(b *Group) {
			cond := Parens(Id("c").Dot("mask0").Op("==").Lit(0))
			for w := 1; w < words; w++ {
				cond = Parens(cond).Op("&&").Parens(
					Id("c").Dot(fmt.Sprintf("mask%d", w)).Op("==").Lit(0),
				)
			}
			b.Return(cond)
		})

	// --- Union
	f.Func().Params(Id("c").Id(coll)).Id("Union").Params(Id("o").Id(coll)).Id(coll).
		BlockFunc(func(b *Group) {
			b.Id("r").Op(":=").Id("c")
			for w := 0; w < words; w++ {
				b.Id("r").Dot(fmt.Sprintf("mask%d", w)).Op("|=").Id("o").Dot(fmt.Sprintf("mask%d", w))
			}
			b.Return(Id("r"))
		})

	// --- Intersect
	f.Func().Params(Id("c").Id(coll)).Id("Intersect").Params(Id("o").Id(coll)).Id(coll).
		BlockFunc(func(b *Group) {
			b.Id("r").Op(":=").Id("c")
			for w := 0; w < words; w++ {
				b.Id("r").Dot(fmt.Sprintf("mask%d", w)).Op("&=").Id("o").Dot(fmt.Sprintf("mask%d", w))
			}
			b.Return(Id("r"))
		})

	// --- Diff
	f.Func().Params(Id("c").Id(coll)).Id("Diff").Params(Id("o").Id(coll)).Id(coll).
		BlockFunc(func(b *Group) {
			b.Id("r").Op(":=").Id("c")
			for w := 0; w < words; w++ {
				b.Id("r").Dot(fmt.Sprintf("mask%d", w)).Op("&^=").Id("o").Dot(fmt.Sprintf("mask%d", w))
			}
			b.Return(Id("r"))
		})

	// --- Contains (o ⊆ c)
	f.Func().Params(Id("c").Id(coll)).Id("Contains").Params(Id("o").Id(coll)).Bool().
		BlockFunc(func(b *Group) {
			cond := Parens(
				Parens(Id("c").Dot("mask0").Op("&").Id("o").Dot("mask0")).Op("==").Id("o").Dot("mask0"),
			)
			for w := 1; w < words; w++ {
				cond = Parens(cond).Op("&&").Parens(
					Parens(Id("c").Dot(fmt.Sprintf("mask%d", w)).Op("&").Id("o").Dot(fmt.Sprintf("mask%d", w))).
						Op("==").Id("o").Dot(fmt.Sprintf("mask%d", w)),
				)
			}
			b.Return(cond)
		})

	// MakeEmpty<Coll>() возвращает пустую коллекцию (все маски = 0)
	f.Func().Id("MakeEmpty" + coll).Params().Id(coll).
		Block(
			Return(Id(coll).Values()), // пустая инициализация обнуляет все maskN
		)

	// --- Make<Coll>All
	f.Func().Id("Make" + coll + "All").Params().Id(coll).
		BlockFunc(func(b *Group) {
			d := Dict{}
			left := count
			for w := 0; w < words; w++ {
				nbits := 64
				if left < 64 {
					nbits = left
				}
				left -= nbits
				field := Id(fmt.Sprintf("mask%d", w))
				switch {
				case nbits == 64:
					d[field] = Op("^").Id("uint64").Call(Lit(0)) // все биты = 1
				case nbits == 0:
					d[field] = Lit(0)
				default:
					d[field] = Parens(Lit(1).Op("<<").Lit(nbits)).Op("-").Lit(1)
				}
			}
			b.Return(Id(coll).Values(d))
		})

	// --- Make<Coll>(vals ...Type)
	f.Func().Id("Make"+coll).Params(Id("vals").Op("...").Id(typeName)).Id(coll).
		Block(
			Id("c").Op(":=").Id(coll).Values(),
			For(List(Id("_"), Id("v")).Op(":=").Range().Id("vals")).Block(
				Switch(Id("v")).BlockFunc(func(sb *Group) {
					for i, v := range vars {
						sb.Case(Id(v.Name)).Block(
							Id("c").Dot(fmt.Sprintf("mask%d", bits[i].word)).
								Op("|=").Parens(Lit(1).Op("<<").Lit(bits[i].bit)),
						)
					}
				}),
			),
			Return(Id("c")),
		)

	// --- String() string  (space-separated; Builder{} и WriteRune)
	f.Func().Params(Id("c").Id(coll)).Id("String").Params().String().
		BlockFunc(func(b *Group) {
			b.Id("sb").Op(":=").Qual("strings", "Builder").Values() // strings.Builder{}
			b.Id("first").Op(":=").True()
			for i := 0; i < count; i++ {
				w := bits[i].word
				bb := bits[i].bit
				constName := fmt.Sprintf("_collgen_%s_%s_Name", typeName, vars[i].Name)
				b.If(
					Parens(
						Id("c").Dot(fmt.Sprintf("mask%d", w)).Op("&").Parens(Lit(1).Op("<<").Lit(bb)),
					).Op("!=").Lit(0),
				).Block(
					If(Op("!").Id("first")).Block(
						Id("sb").Dot("WriteRune").Call(Lit(' ')),
					),
					Id("first").Op("=").False(),
					Id("sb").Dot("WriteString").Call(Id(constName)),
				)
			}
			b.Return(Id("sb").Dot("String").Call())
		})

	// --- Parse<Coll>(s string) (switch по константам)
	f.Func().Id("Parse"+coll).
		Params(Id("s").String()).
		Params(Id(coll), Error()).
		Block(
			Id("c").Op(":=").Id(coll).Values(),
			For(List(Id("_"), Id("tok")).Op(":=").Range().Qual("strings", "Fields").Call(Id("s"))).Block(
				Switch(Id("tok")).BlockFunc(func(sb *Group) {
					for i, v := range vars {
						constName := fmt.Sprintf("_collgen_%s_%s_Name", typeName, v.Name)
						sb.Case(Id(constName)).Block(
							// установим соответствующий бит (через Add, чтобы логика была единообразной)
							Id("c").Op("=").Id("c").Dot("Add").Call(Id(v.Name)),
						)
						_ = i
					}
					sb.Default().Block(
						Return(Id("c"), Qual("fmt", "Errorf").Call(Lit("unknown %s: %q"), Lit(typeName), Id("tok"))),
					)
				}),
			),
			Return(Id("c"), Nil()),
		)

	// --- JSON: MarshalJSON/UnmarshalJSON
	// MarshalJSON кодирует коллекцию как JSON-строку вида "name1 name2 ..."
	f.Func().
		Params(Id("c").Id(coll)).
		Id("MarshalJSON").
		Params().
		Params(Index().Byte(), Error()).
		Block(
			Return(
				Qual("encoding/json", "Marshal").
					Call(Id("c").Dot("String").Call()),
			),
		)

	// UnmarshalJSON ожидает JSON-строку вида "name1 name2 ..."
	f.Func().
		Params(Id("c").Op("*").Id(coll)).
		Id("UnmarshalJSON").
		Params(Id("b").Index().Byte()).
		Error().
		Block(
			Var().Id("s").String(),
			If(Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(Id("b"), Op("&").Id("s")), Err().Op("!=").Nil()).
				Block(Return(Err())),
			List(Id("v"), Err()).Op(":=").Id("Parse"+coll).Call(Id("s")),
			If(Err().Op("!=").Nil()).Block(Return(Err())),
			Op("*").Id("c").Op("=").Id("v"),
			Return(Nil()),
		)
}
