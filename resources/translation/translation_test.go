package translation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryLanguageSaysEverything holds the languages to one another: a code
// one of them has no words for reaches its readers as an empty message, which
// says less than the code itself.
func TestEveryLanguageSaysEverything(t *testing.T) {
	t.Parallel()

	for locale, words := range Translations {
		for other, otherWords := range Translations {
			for key := range otherWords {
				assert.Contains(t, words, key, "%s says %q and %s does not", other, key, locale)
			}
		}

		for key, said := range words {
			assert.NotEmpty(t, said, "%s says %q as nothing", locale, key)
		}
	}
}

// TestTheWorkloadsCodesAreSaid holds the codes the workload answers with to
// having words: the dashboard's own rules, the control plane's refusals and a
// node's, which reach a reader as they are said here.
func TestTheWorkloadsCodesAreSaid(t *testing.T) {
	t.Parallel()

	codes := []string{
		// the dashboard's
		"invalid_kind", "invalid_access", "invalid_port", "duplicate_port", "too_many_ports",
		"invalid_lifetime", "invalid_image", "image_of_another_kind", "invalid_name", "invalid_protocol",
		"invalid_mount_type", "invalid_mount", "invalid_restart_policy",
		"invalid_network_driver", "invalid_volume_driver", "vm_or_new_vm",

		// the control plane's
		"too_large", "too_small", "quota_exceeded", "no_capacity",
		"engine_mismatch", "kind_mismatch", "snapshot_not_ready", "disk_too_small",
		"disk_cannot_shrink", "immutable", "vm_not_running", "node_lost", "managed_by_code_runner",

		// a node's
		"not_found", "not_running", "not_docker", "docker_unavailable", "invalid", "timeout", "internal",
	}

	for locale, words := range Translations {
		for _, code := range codes {
			assert.NotEmpty(t, words[code], "%s has no words for %q", locale, code)
		}
	}
}

// controlPlane is where the workload's control plane decides what it answers
// with: its use cases, and the handlers of its API.
var controlPlane = []string{
	"../../application/workload/controlplane",
	"../../presentation/http/workload/controlplane",
}

// codeShape is what a code looks like: a word, or words joined by underscores.
var codeShape = regexp.MustCompile(`^[a-z]+(_[a-z0-9]+)*$`)

// TestTheControlPlanesCodesAreSaid holds every code the control plane answers
// with to having words, read out of the control plane itself rather than
// listed again here, so a code added there is held to it as soon as it is.
//
// The control plane refuses in codes, which the blog puts into the words of
// whoever asked, and a VM or a stack waiting or lost says why in one; a code
// there are no words for reaches its reader as the code itself.
func TestTheControlPlanesCodesAreSaid(t *testing.T) {
	t.Parallel()

	codes := controlPlaneCodes(t)
	require.NotEmpty(t, codes)

	for locale, words := range Translations {
		for code, where := range codes {
			assert.NotEmpty(t, words[code], "%s has no words for %q, which %s answers with", locale, code, where)
		}
	}
}

// controlPlaneCodes is every code the control plane answers with, and where
// it first says it. A code is what it refuses a field with, a value it returns
// alongside or in place of one, what it translates, and what it keeps as a
// Code, Reason or Note constant.
func controlPlaneCodes(t *testing.T) map[string]string {
	t.Helper()

	codes := make(map[string]string)

	for _, root := range controlPlane {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}

			files := token.NewFileSet()

			parsed, err := parser.ParseFile(files, path, nil, 0)
			if err != nil {
				return err
			}

			keep := func(expression ast.Expr) {
				literal, ok := expression.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return
				}

				value, err := strconv.Unquote(literal.Value)
				if err != nil || !codeShape.MatchString(value) {
					return
				}

				if _, known := codes[value]; !known {
					codes[value] = files.Position(literal.Pos()).String()
				}
			}

			ast.Inspect(parsed, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.CompositeLit:
					// domain.ValidationErrors{"field": "code"}
					if named, ok := node.Type.(*ast.SelectorExpr); ok && named.Sel.Name == "ValidationErrors" {
						for _, element := range node.Elts {
							if pair, ok := element.(*ast.KeyValueExpr); ok {
								keep(pair.Value)
							}
						}
					}

				case *ast.AssignStmt:
					// validationErrors["field"] = "code"
					for i, assigned := range node.Lhs {
						if _, ok := assigned.(*ast.IndexExpr); ok && i < len(node.Rhs) {
							keep(node.Rhs[i])
						}
					}

				case *ast.ReturnStmt:
					// return "code", false
					for _, result := range node.Results {
						keep(result)
					}

				case *ast.CallExpr:
					// translator.Translate("code")
					if called, ok := node.Fun.(*ast.SelectorExpr); ok && called.Sel.Name == "Translate" && len(node.Args) > 0 {
						keep(node.Args[0])
					}

				case *ast.ValueSpec:
					// const ReasonNodeLost = "node_lost"
					for i, name := range node.Names {
						if i >= len(node.Values) {
							continue
						}

						for _, prefix := range []string{"Code", "Reason", "Note"} {
							if strings.HasPrefix(name.Name, prefix) {
								keep(node.Values[i])
							}
						}
					}
				}

				return true
			})

			return nil
		})
		require.NoError(t, err)
	}

	return codes
}
