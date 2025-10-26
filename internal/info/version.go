// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package info

import (
	"runtime/debug"
)

var (
	// NOTE: The $Format strings are replaced during 'git archive' thanks to the
	// companion .gitattributes file containing 'export-subst' in this same
	// directory.  See also https://git-scm.com/docs/gitattributes
	gitVersion   = "v0.0.1-unreleased" // "v0.0.0-master+$Format:%h$"
	gitCommit    = ""                  // sha1 from git, output of $(git rev-parse HEAD)
	gitTreeState = ""                  // state of git tree, either "clean" or "dirty"

	buildDate   = "2006-01-02T15:04:05Z" // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	environment = "local"
)

func init() {
	info, ok := debug.ReadBuildInfo()
	if ok {
		if info.Main.Version != "" {
			gitVersion = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				gitCommit = setting.Value
			case "vcs.time":
				buildDate = setting.Value
			case "vcs.modified":
				gitTreeState = setting.Value
			}
		}
	}

	Version = version{
		BuildDate:    buildDate,
		Environment:  environment,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
		GitVersion:   gitVersion,
	}
}
