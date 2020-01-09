// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	binaryBaseName = "pulse"
)

var (
	Default = Build

	projectRoot = mustGetProjectRoot()
	binaryDir   = filepath.Join(projectRoot, "bin")

	targetPlatforms = []Platform{
		{"linux", "amd64"},
		{"darwin", "amd64"},
	}
)

// BuildFast compiles the project. Formatting and tests are skipped.
func BuildFast() error {
	mg.Deps(CheckDeps)
	return buildAll()
}

// Build compiles the project from scratch. Tests are run before the build is executed.
func Build() error {
	mg.Deps(CheckDeps, Clean, Format, Test)
	return buildAll()
}

// Clean removes any artifacts generated from the build process.
func Clean() {
	_ = os.RemoveAll(binaryDir)
}

// CheckDeps checks to see if necessary tools to build are installed
func CheckDeps() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf(`dependency not found: git

on macOS:         brew install git
on Ubuntu/Debian: apt-get install git
on RedHat/Fedora: dnf install git
`)
	}

	if _, err := exec.LookPath("goimports"); err != nil {
		return fmt.Errorf(`dependency not found: goimports

go get golang.org/x/tools/cmd/goimports
`)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("dependency not found: go")
	}

	// TODO: should add a minimum Go version check

	return nil
}

// Generate runs go generate to run any commands described by generate directives after build
func Generate() {
	_ = sh.Run("go", "generate", "./...")
}

// Format runs goimports and go fmt to ensure standard go formatting rules are followed.
func Format() {
	mg.Deps(CheckDeps)

	_ = sh.Run("goimports", "-w", ".")
	_ = sh.Run("go", "fmt", "./...")
}

// Test runs unit tests.
func Test() error {
	mg.Deps(CheckDeps)

	_, err := sh.Exec(nil, os.Stdout, os.Stderr, "go", "test", "-v", "./...")
	return err
}

// Version outputs the version of the build.
func Version() error {
	mg.Deps(CheckDeps)

	version, err := getVersion()
	if err != nil {
		return err
	}

	fmt.Println(version)
	return nil
}

func buildAll() error {
	buildTime := time.Now().UTC().Format(time.RFC3339)
	commit, err := getCommit()
	if err != nil {
		return err
	}

	version, err := getVersion()
	if err != nil {
		return err
	}

	for _, p := range targetPlatforms {
		buildInfo := BuildInfo{
			Platform:  p,
			Commit:    commit,
			Version:   version,
			BuildTime: buildTime,
		}

		if err := buildPlatform(buildInfo); err != nil {
			return err
		}
	}

	// generate a convenient symlink in bin/ for the target platform so developers can invoke bin/$PROGRAM
	currentOS := runtime.GOOS
	currentCPU := runtime.GOARCH

	fullBinaryName := fmt.Sprintf("%s-%s-%s", binaryBaseName, currentOS, currentCPU)
	fullBinaryPath := filepath.Join(projectRoot, "bin", fullBinaryName)

	if err := os.Symlink(fullBinaryPath, filepath.Join(projectRoot, "bin", binaryBaseName)); err != nil {
		if os.IsExist(err) {
			return nil
		}

		return err
	}

	return nil
}

func buildPlatform(b BuildInfo) error {
	buildEnv := map[string]string{
		"GOOS":   b.OSFamily,
		"GOARCH": b.CPUArchitecture,
	}

	ldFlags := []string{
		fmt.Sprintf("-X goms.io/aks/%s/pkg/version.ReleaseVersion=%s", binaryBaseName, b.Version),
		fmt.Sprintf("-X goms.io/aks/%s/pkg/version.GitSHA=%s", binaryBaseName, b.Commit),
		fmt.Sprintf("-X goms.io/aks/%s/pkg/version.BuildTime=%s", binaryBaseName, b.BuildTime),
	}

	ldFlagsArg := fmt.Sprintf("-ldflags=%s", strings.Join(ldFlags, " "))
	outputPath := filepath.Join(binaryDir, fmtBinaryName(b.OSFamily, b.CPUArchitecture))
	buildArgs := []string{"build", "-o", outputPath, ldFlagsArg, "main.go"}

	_, err := sh.Exec(buildEnv, os.Stdout, os.Stdout, "go", buildArgs...)
	if err != nil {
		return err
	}

	return nil
}

type Platform struct {
	OSFamily        string
	CPUArchitecture string
}

type BuildInfo struct {
	Platform
	Version   string
	Commit    string
	BuildTime string
}

func fmtBinaryName(osFamily, cpuArchitecture string) string {
	return fmt.Sprintf("%s-%s-%s", binaryBaseName, osFamily, cpuArchitecture)
}

func getCommit() (string, error) {
	res, err := sh.Output("git", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return res, nil
}

func getVersion() (string, error) {
	res, err := sh.Output("git", "describe", "--tags", "--always", "--abbrev=0")
	if err != nil {
		return "", err
	}

	return res, nil
}

// getProjectRoot invokes git and returns the parent of the .git directory which is effectively the "project root".
func mustGetProjectRoot() string {
	// using exec rather than mage/sh because of weird logging issue with mage/sh and var () block at top.
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	res, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}

	return filepath.Clean(strings.TrimSpace(string(res)))
}
