package spec

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

const Version = "2020-08-01"

type Test struct {
	Name        string
	Description string
	InputYAML   []byte
	InputJSON   []byte
	OutputYAML  []byte
	IsError     bool
}

func readFile(fs billy.Filesystem, path string) ([]byte, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ioutil.ReadAll(f)
}

func exists(fs billy.Filesystem, path string) bool {
	_, err := fs.Stat(path)
	return err == nil
}

func loadTest(dir billy.Filesystem, name string) (Test, error) {
	description, err := readFile(dir, "===")
	if err != nil && !os.IsNotExist(err) {
		return Test{}, fmt.Errorf("loading description: %w", err)
	}
	inputYAML, err := readFile(dir, "in.yaml")
	if err != nil && !os.IsNotExist(err) {
		return Test{}, fmt.Errorf("loading input YAML: %w", err)
	}
	inputJSON, err := readFile(dir, "in.json")
	if err != nil && !os.IsNotExist(err) {
		return Test{}, fmt.Errorf("loading input JSON: %w", err)
	}
	outputYAML, err := readFile(dir, "out.yaml")
	if err != nil && !os.IsNotExist(err) {
		return Test{}, fmt.Errorf("loading output YAML: %w", err)
	}
	return Test{
		Name:        name,
		Description: string(description),
		InputYAML:   inputYAML,
		InputJSON:   inputJSON,
		OutputYAML:  outputYAML,
		IsError:     exists(dir, "error"),
	}, nil
}

func LoadTest(path string) (Test, error) {
	_, err := os.Stat(path)
	if err != nil {
		return Test{}, err
	}

	return loadTest(osfs.New(path), filepath.Base(path))
}

func loadTests(dir billy.Filesystem) ([]Test, error) {
	entries, err := dir.ReadDir("/")
	if err != nil {
		return nil, err
	}
	var tests []Test
	for _, info := range entries {
		if !info.IsDir() || info.Name() == "meta" || info.Name() == "names" || info.Name() == "tags" {
			continue
		}

		test, err := loadTest(chroot.New(dir, info.Name()), info.Name())
		if err != nil {
			return nil, fmt.Errorf("loading test %v: %w", info.Name(), err)
		}
		tests = append(tests, test)
	}
	sort.Slice(tests, func(i, j int) bool { return tests[i].Name < tests[j].Name })
	return tests, nil
}

func LoadTests(path string) ([]Test, error) {
	return loadTests(osfs.New(path))
}

func LoadLatestTests() ([]Test, error) {
	fs := memfs.New()
	storage := memory.NewStorage()
	_, err := git.Clone(storage, fs, &git.CloneOptions{
		URL:           "https://github.com/yaml/yaml-test-suite.git",
		ReferenceName: plumbing.NewTagReferenceName("data-" + Version),
		SingleBranch:  true,
	})
	if err != nil {
		return nil, err
	}
	return loadTests(fs)
}
