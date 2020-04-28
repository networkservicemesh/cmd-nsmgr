package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/networkservicemesh/sdk/pkg/tools/executils"
	"github.com/sirupsen/logrus"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

// FindMainPackages -
func FindMainPackages(ctx context.Context, env []string) []string {
	roots := []string{}
	curDir, err := os.Getwd()
	if err != nil {
		logrus.Errorf("Failed to receive current dir %v", err)
	}
	// Find a list of package roots using
	var listResult []byte
	//List commands do not need to pass environment
	listCmd := fmt.Sprintf("go list -f {{.Name}}:{{.Dir}} ./...")
	listResult, err = executils.Output(ctx, listCmd, executils.WithEnviron(env))
	if err != nil {
		logrus.Fatalf("failed to find a list of go package roots: %v", err)
	}
	lines := strings.Split(string(listResult), "\n")
	for _, line := range lines {
		trimLine := strings.TrimSpace(line)
		if len(trimLine) == 0 {
			continue
		}
		if strings.HasPrefix(trimLine, "main:") {
			// we found main package, let's add it as root
			roots = append(roots, trimLine[len(curDir)+6:len(trimLine)])
		}
	}
	return roots
}

func BuildRelaivePath(rootDir, pkgName string) string {
	ind := strings.Index(pkgName, rootDir)
	if ind != -1 {
		relPath := pkgName[ind+len(rootDir) : len(pkgName)]
		if strings.HasPrefix(relPath, "/") {
			relPath = relPath[1:len(relPath)]
		}
		return relPath
	}
	return pkgName
}

type PackageInfo struct {
	RelPath string
	Tests   []string
	OutName string
}

type TestEvent struct {
	Time    time.Time // encodes as an RFC3339-format string
	Action  string
	Package string
	Test    string
	Elapsed float64 // seconds
	Output  string
}

var alphaReg, _ = regexp.Compile("[^A-Za-z0-9]+")

func FindTests(ctx context.Context, rootDir string, env []string) (map[string]*PackageInfo, error) {
	logrus.Infof("Find Tests in %v", rootDir)
	testPackages := map[string]*PackageInfo{}
	// Find all Tests
	// List commands do not need to pass environment
	listCmd := fmt.Sprintf("go test %v/... --list .* -json", "./"+rootDir)
	listResult, err := executils.Output(ctx, listCmd, executils.WithEnviron(env))
	if err != nil {
		logrus.Errorf("Failed to list Tests %v %v", listCmd, err)
		return nil, err
	}

	_, cmdName := path.Split(path.Clean(rootDir))

	lines := strings.Split(string(listResult), "\n")
	for _, line := range lines {
		trimLine := strings.TrimSpace(line)
		if len(trimLine) == 0 {
			continue
		}
		event := TestEvent{}
		err := json.Unmarshal([]byte(trimLine), &event)
		if err != nil {
			logrus.Errorf("Failed to parse line: %v", err)
			continue
		}
		pkgInfo, ok := testPackages[event.Package]
		if !ok {
			relPath := BuildRelaivePath(rootDir, event.Package)
			outName := fmt.Sprintf("%s-%s.test", cmdName, alphaReg.ReplaceAllString(relPath, "-"))
			if len(relPath) == 0 {
				outName = fmt.Sprintf("%s.test", cmdName)
			}
			pkgInfo = &PackageInfo{
				RelPath: relPath,
				OutName: outName,
				Tests:   []string{},
			}
			testPackages[event.Package] = pkgInfo
		}

		switch event.Action {
		case "output":
			for _, k := range strings.Split(strings.TrimSpace(event.Output), "\n") {
				if strings.HasPrefix(k, "Test") {
					pkgInfo.Tests = append(pkgInfo.Tests, k)
				}
			}
		case "skip":
			pkgInfo.Tests = []string{}
		}
	}
	return testPackages, nil
}
