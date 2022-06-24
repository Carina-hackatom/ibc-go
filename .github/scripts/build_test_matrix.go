package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	testNamePrefix     = "Test"
	testFileNameSuffix = "_test.go"
)

// GithubActionTestMatrix represents
type GithubActionTestMatrix struct {
	Include []TestSuitePair `json:"include"`
}

type TestSuitePair struct {
	Test  string `json:"test"`
	Suite string `json:"suite"`
}

func main() {
	githubActionMatrixJsonString, err := getGithubActionMatrixForTests("e2e")
	if err != nil {
		fmt.Sprintf("error generating github action json: %s", err)
		os.Exit(1)
	}
	fmt.Println(githubActionMatrixJsonString)
}

// getGithubActionMatrixForTests returns a json string representing the contents that should go in the matrix
// field in a github action workflow. This string can be used with `fromJSON(str)` to dynamically build
// the workflow matrix to include all E2E tests under the rootPath directory.
func getGithubActionMatrixForTests(rootPath string) (string, error) {
	testSuiteMapping := map[string][]string{}
	fset := token.NewFileSet()
	err := filepath.Walk(rootPath, func(path string, info fs.FileInfo, err error) error {
		// only look at test files
		if !strings.HasSuffix(path, testFileNameSuffix) {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("failed parsing file: %s", err)
		}

		suiteNameForFile, testCases, err := extractSuiteAndTestNames(f)
		if err != nil {
			return fmt.Errorf("failed extracting test suite name and test cases: %s", err)
		}

		testSuiteMapping[suiteNameForFile] = testCases

		return nil
	})

	if err != nil {
		return "", err
	}

	gh := GithubActionTestMatrix{
		Include: []TestSuitePair{},
	}

	for testSuiteName, testCases := range testSuiteMapping {
		for _, testCaseName := range testCases {
			gh.Include = append(gh.Include, TestSuitePair{
				Test:  testCaseName,
				Suite: testSuiteName,
			})
		}
	}

	ghBytes, err := json.Marshal(gh)
	if err != nil {
		return "", err
	}

	return string(ghBytes), nil
}

// extractSuiteAndTestNames extracts the name of the test suite function as well
// as all tests associated with it in the same file.
func extractSuiteAndTestNames(file *ast.File) (string, []string, error) {
	var suiteNameForFile string
	var testCases []string

	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok {
			functionName := f.Name.Name
			if isTestSuiteMethod(f) {
				suiteNameForFile = functionName
				continue
			}
			if isTestFunction(f) {
				testCases = append(testCases, functionName)
			}
		}
	}
	if suiteNameForFile == "" {
		return "", nil, fmt.Errorf("file %s had no test suite test case", file.Name.Name)
	}
	return suiteNameForFile, testCases, nil
}

// isTestSuiteMethod returns true if the function is a test suite function.
// e.g. func TestFeeMiddlewareTestSuite(t *testing.T) { ... }
func isTestSuiteMethod(f *ast.FuncDecl) bool {
	return strings.HasPrefix(f.Name.Name, testNamePrefix) && len(f.Type.Params.List) == 1
}

// isTestFunction returns true if the function name starts with "Test" and has no parameters.
// as test suite functions to not accept a testing.T.
func isTestFunction(f *ast.FuncDecl) bool {
	return strings.HasPrefix(f.Name.Name, testNamePrefix) && len(f.Type.Params.List) == 0
}