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
	"testing"
)

func TestGc(t *testing.T) {
	t.Parallel()
	f := makeDumbcasAppMock(t)

	args := []string{"gc", "-root=\\"}
	f.Run(args, 0)

	items := EnumerateAsList(f.TB, f.cas)
	f.Assertf(len(items) == 0, "Unexpected items: %s", items)

	// Create a tree of stuff.
	tree1 := map[string]string{
		"file1":           "content1",
		"dir1/dir2/file2": "content2",
	}
	archiveData(f.TB, f.cas, f.nodes, tree1)

	args = []string{"gc", "-root=\\"}
	f.Run(args, 0)
	items = EnumerateAsList(f.TB, f.cas)
	f.Assertf(len(items) == 3, "Unexpected items: %s", items)
}
