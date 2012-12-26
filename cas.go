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
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
)

const casName = "cas"
const needFsckName = "need_fsck"
const hashLength = 40

type CasEntry struct {
	Item  string
	Error error
}

// Creates 16^3 (4096) directories. Preferable values are 2 or 3.
const splitAt = 3

type casTable struct {
	rootDir      string
	casDir       string
	prefixLength int
	validPath    *regexp.Regexp
	trash        Trash
}

type CasTable interface {
	// Serves the table over HTTP GET interface.
	http.Handler
	// Enumerates all the entries in the table.
	Enumerate() <-chan CasEntry
	// Add an entry to the table.
	AddEntry(source io.Reader, hash string) error
	// Opens an entry for reading.
	Open(hash string) (ReadSeekCloser, error)
	// Removes an entry in the table.
	Remove(item string) error
	// Sets the bit that the table needs to be checked for consistency.
	NeedFsck()
	// Returns if the fsck bit is set.
	WarnIfFsckIsNeeded() bool
}

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// Converts an entry in the table into a proper file path.
func (c *casTable) filePath(hash string) string {
	match := c.validPath.FindStringSubmatch(hash)
	if match == nil {
		log.Printf("filePath(%s) is invalid", hash)
		return ""
	}
	fullPath := path.Join(c.casDir, match[0][:c.prefixLength], match[0][c.prefixLength:])
	if !path.IsAbs(fullPath) {
		log.Printf("filePath(%s) is invalid", hash)
		return ""
	}
	return fullPath
}

func prefixSpace(prefixLength uint) int {
	if prefixLength == 0 {
		return 0
	}
	return 1 << (prefixLength * 4)
}

func MakeCasTable(rootDir string) (CasTable, error) {
	//log.Printf("MakeCasTable(%s)", rootDir)
	if !path.IsAbs(rootDir) {
		return nil, fmt.Errorf("MakeCasTable(%s) is not valid", rootDir)
	}
	rootDir = path.Clean(rootDir)
	casDir := path.Join(rootDir, casName)
	prefixLength := splitAt
	if err := os.Mkdir(casDir, 0750); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("MakeCasTable(%s): failed to create %s: %s", casDir, err)
	} else if !os.IsExist(err) {
		// Create all the prefixes at initialization time so they don't need to be
		// tested all the time.
		for i := 0; i < prefixSpace(uint(prefixLength)); i++ {
			prefix := fmt.Sprintf("%0*x", prefixLength, i)
			if err := os.Mkdir(path.Join(casDir, prefix), 0750); err != nil && !os.IsExist(err) {
				return nil, fmt.Errorf("Failed to create %s: %s\n", prefix, err)
			}
		}
	}
	return &casTable{
		rootDir,
		casDir,
		prefixLength,
		regexp.MustCompile(fmt.Sprintf("^([a-f0-9]{%d})$", hashLength)),
		MakeTrash(casDir),
	}, nil
}

// Expects the format "/<hash>". In particular, refuses "/<hash>/".
func (c *casTable) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//log.Printf("casTable.ServeHTTP(%s)", r.URL.Path)
	if r.URL.Path == "" || r.URL.Path[0] != '/' {
		http.Error(w, "Internal failure. CasTable received an invalid url: "+r.URL.Path, http.StatusNotImplemented)
		return
	}
	casItem := c.filePath(r.URL.Path[1:])
	if casItem == "" {
		http.Error(w, "Invalid CAS url: "+r.URL.Path, http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, casItem)
}

// Enumerates all the entries in the table. If a file or directory is found in
// the directory tree that doesn't match the expected format, it will be moved
// into the trash.
func (c *casTable) Enumerate() <-chan CasEntry {
	rePrefix := regexp.MustCompile(fmt.Sprintf("^[a-f0-9]{%d}$", c.prefixLength))
	reRest := regexp.MustCompile(fmt.Sprintf("^[a-f0-9]{%d}$", hashLength-c.prefixLength))
	items := make(chan CasEntry)

	// TODO(maruel): No need to read all at once.
	go func() {
		prefixes, err := readDirNames(c.casDir)
		if err != nil {
			items <- CasEntry{Error: fmt.Errorf("Failed reading ss", c.casDir)}
		} else {
			for _, prefix := range prefixes {
				if IsInterrupted() {
					break
				}
				if prefix == TrashName {
					continue
				}
				if !rePrefix.MatchString(prefix) {
					c.trash.Move(prefix)
					c.NeedFsck()
					continue
				}
				// TODO(maruel): No need to read all at once.
				prefixPath := path.Join(c.casDir, prefix)
				subitems, err := readDirNames(prefixPath)
				if err != nil {
					items <- CasEntry{Error: fmt.Errorf("Failed reading %s", prefixPath)}
					c.NeedFsck()
					continue
				}
				for _, item := range subitems {
					if !reRest.MatchString(item) {
						c.trash.Move(path.Join(prefix, item))
						c.NeedFsck()
						continue
					}
					items <- CasEntry{Item: prefix + item}
				}
			}
		}
		close(items)
	}()
	return items
}

// Adds an entry with the hash calculated already if not alreaady present. It's
// a performance optimization to be able to not write the object unless needed.
func (c *casTable) AddEntry(source io.Reader, hash string) error {
	dst := c.filePath(hash)
	df, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
	if os.IsExist(err) {
		return err
	}
	if err != nil {
		return fmt.Errorf("Failed to copy(dst) %s: %s", dst, err)
	}
	defer df.Close()
	_, err = io.Copy(df, source)
	return err
}

func (c *casTable) Open(hash string) (ReadSeekCloser, error) {
	fp := c.filePath(hash)
	if fp == "" {
		return nil, os.ErrInvalid
	}
	return os.Open(fp)
}

// Signals that an fsck is required.
func (c *casTable) NeedFsck() {
	log.Printf("Marking for fsck")
	f, _ := os.Create(path.Join(c.casDir, needFsckName))
	if f != nil {
		f.Close()
	}
}

func (c *casTable) WarnIfFsckIsNeeded() bool {
	f, _ := os.Open(path.Join(c.casDir, needFsckName))
	if f == nil {
		return false
	}
	defer f.Close()
	fmt.Fprintf(os.Stderr, "WARNING: fsck is needed.")
	return true
}

func (c *casTable) Remove(hash string) error {
	match := c.validPath.FindStringSubmatch(hash)
	if match == nil {
		return fmt.Errorf("Remove(%s) is invalid", hash)
	}
	return c.trash.Move(path.Join(hash[:c.prefixLength], hash[c.prefixLength:]))
}

// Utility function when the data is already in memory but not yet hashed.
func AddBytes(c CasTable, data []byte) (string, error) {
	hash := sha1Bytes(data)
	return hash, c.AddEntry(bytes.NewBuffer(data), hash)
}
