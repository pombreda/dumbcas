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
	"fmt"
)

var cmdGc = &Command{
	UsageLine: "gc",
	ShortDesc: "moves to trash all objects that are not referenced anymore",
	LongDesc:  "Scans each node and each entry file to determine if each cas entry is referenced or not.",
	CommandRun: func() CommandRun {
		c := &gcRun{}
		c.Init()
		return c
	},
}

type gcRun struct {
	CommonFlags
}

func TagRecurse(entries map[string]bool, entry *Entry) {
	if entry.Sha1 != "" {
		entries[entry.Sha1] = true
	}
	for _, i := range entry.Files {
		TagRecurse(entries, i)
	}
}

func (c *gcRun) main(a DumbcasApplication) error {
	if err := c.Parse(a, false, false); err != nil {
		return err
	}

	entries := map[string]bool{}
	for item := range c.cas.Enumerate() {
		if item.Error != nil {
			// TODO(maruel): Leaks channel.
			c.cas.SetFsckBit()
			return fmt.Errorf("Failed enumerating the CAS table %s", item.Error)
		}
		entries[item.Item] = false
	}
	a.GetLog().Printf("Found %d entries", len(entries))

	// Load all the nodes.
	for item := range c.nodes.Enumerate() {
		if item.Error != nil {
			// TODO(maruel): Leaks channel.
			return item.Error
		}
		entries[item.Node.Entry] = true
		entry, err := LoadEntry(c.cas, item.Node.Entry)
		if err != nil {
			return err
		}
		TagRecurse(entries, entry)
	}

	orphans := []string{}
	for entry, tagged := range entries {
		if !tagged {
			orphans = append(orphans, entry)
		}
	}
	a.GetLog().Printf("Found %d orphan", len(orphans))
	for _, orphan := range orphans {
		if err := c.cas.Remove(orphan); err != nil {
			c.cas.SetFsckBit()
			return fmt.Errorf("Internal error while removing %s: %s", orphan, err)
		}
	}
	return nil
}

func (c *gcRun) Run(a Application, args []string) int {
	if len(args) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: Unsupported arguments.\n", a.GetName())
		return 1
	}
	d := a.(DumbcasApplication)
	if err := c.main(d); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
