/* Copyright 2012 Marc-Antoine Ruel. Licensed under the Apache License, Version
2.0 (the "License"); you may not use this file except in compliance with the
License.  You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0. Unless required by applicable law or
agreed to in writing, software distributed under the License is distributed on
an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express
or implied. See the License for the specific language governing permissions and
limitations under the License. */

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

// Common flags.
var Root string

func GetCommonFlags() flag.FlagSet {
	flags := flag.FlagSet{}
	flags.StringVar(&Root, "root", os.Getenv("DUMBCAS_ROOT"), "Root directory; required. Set $DUMBCAS_ROOT to set a default.")
	return flags
}

func CommonFlag(createRoot bool, bypassFsck bool) (CasTable, error) {
	if Root == "" {
		return nil, errors.New("Must provide -root")
	}
	if root, err := filepath.Abs(Root); err != nil {
		return nil, fmt.Errorf("Failed to find %s", Root)
	} else {
		Root = root
	}

	if createRoot {
		if err := os.MkdirAll(Root, 0750); err != nil && !os.IsExist(err) {
			return nil, fmt.Errorf("Failed to create %s: %s", Root, err)
		}
	}

	cas, err := MakeCasTable(Root)
	if err != nil {
		return nil, err
	}
	if cas.WarnIfFsckIsNeeded() && !bypassFsck {
		return nil, fmt.Errorf("Can't run if fsck is needed. Please run fsck first.")
	}
	return cas, nil
}

type TreeItem struct {
	FullPath string
	os.FileInfo
	Error error
}

func recurseEnumerateTree(rootDir string, c chan<- TreeItem) bool {
	f, err := os.Open(rootDir)
	if err != nil {
		c <- TreeItem{Error: err}
		return false
	}
	defer f.Close()
	for {
		if IsInterrupted() {
			break
		}
		dirs, err := f.Readdir(1024)
		if err != nil && err != io.EOF {
			c <- TreeItem{Error: err}
			return false
		}
		if len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			if IsInterrupted() {
				break
			}
			name := d.Name()
			fullPath := path.Join(rootDir, name)
			if d.IsDir() {
				if !recurseEnumerateTree(fullPath, c) {
					return false
				}
			} else {
				c <- TreeItem{FullPath: fullPath, FileInfo: d}
			}
		}
	}
	return true
}

// Walk the directory tree.
func EnumerateTree(rootDir string, c chan<- TreeItem) {
	recurseEnumerateTree(rootDir, c)
	close(c)
}

func isDir(path string) bool {
	stat, _ := os.Stat(path)
	return stat != nil && stat.IsDir()
}

// Reads a directory list and guarantees to return a list.
func readDirNames(dirPath string) ([]string, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return []string{}, err
	}
	defer f.Close()
	return f.Readdirnames(0)
}

// Reads a directory list and guarantees to return a list.
func readDirFancy(dirPath string) ([]string, error) {
	names := []string{}
	f, err := os.Open(dirPath)
	if err != nil {
		return names, err
	}
	defer f.Close()
	for {
		dirs, err := f.Readdir(1024)
		if err != nil || len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			name := d.Name()
			if d.IsDir() {
				name += "/"
			}
			names = append(names, name)
		}
	}
	return names, err
}

func sha1File(f io.Reader) (string, error) {
	hash := sha1.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sha1FilePath(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return sha1File(f)
}

func sha1Bytes(content []byte) string {
	hash := sha1.New()
	hash.Write(content)
	return hex.EncodeToString(hash.Sum(nil))
}

func sha1String(content string) string {
	hash := sha1.New()
	io.WriteString(hash, content)
	return hex.EncodeToString(hash.Sum(nil))
}

func loadReaderAsJson(r io.Reader, value interface{}) error {
	data, err := ioutil.ReadAll(r)
	if err == nil {
		return json.Unmarshal(data, &value)
	}
	return err
}

func loadFileAsJson(filepath string, value interface{}) error {
	f, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("loadFileAsJson(%s): %s", filepath, err)
	}
	defer f.Close()
	return loadReaderAsJson(f, value)
}
