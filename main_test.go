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
	"log"
	"testing"
)

type DumbcasAppMock struct {
	*ApplicationMock
	// Statefullness
	cache *mockCache
	cas   CasTable
	nodes NodesTable
}

func (a *DumbcasAppMock) GetLog() *log.Logger {
	return a.log
}

func (a *DumbcasAppMock) Run(args []string, expected int) {
	a.GetLog().Printf("%s", args)
	returncode := Run(a, args)
	a.Assertf(returncode == expected, "Unexpected return code %d", returncode)
}

func makeDumbcasAppMock(t *testing.T, verbose bool) *DumbcasAppMock {
	a := &DumbcasAppMock{
		ApplicationMock: MakeAppMock(t, verbose),
	}
	return a
}
