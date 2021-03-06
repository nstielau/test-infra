/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clonerefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/kube"
)

// Options configures the clonerefs tool
// completely and may be provided using JSON
// or user-specified flags, but not both.
type Options struct {
	// SrcRoot is the root directory under which
	// all source code is cloned
	SrcRoot string `json:"src_root"`
	// Log is the log file to which clone records are written
	Log string `json:"log"`

	// GitUserName is an optional field that is used with
	// `git config user.name`
	GitUserName string `json:"git_user_name,omitempty"`
	// GitUserEmail is an optional field that is used with
	// `git config user.email`
	GitUserEmail string `json:"git_user_email,omitempty"`

	// GitRefs are the refs to clone
	GitRefs []*kube.Refs `json:"refs"`

	// MaxParallelWorkers determines how many repositories
	// can be cloned in parallel. If 0, interpreted as no
	// limit to parallelism
	MaxParallelWorkers int `json:"max_parallel_workers,omitempty"`

	// used to hold flag values
	refs    gitRefs
	aliases pathAliases
}

// Validate ensures that the configuration options are valid
func (o *Options) Validate() error {
	if o.SrcRoot == "" {
		return errors.New("no source root specified")
	}

	if o.Log == "" {
		return errors.New("no log file specified")
	}

	if len(o.GitRefs) == 0 {
		return errors.New("no refs specified to clone")
	}

	seen := map[string]sets.String{}
	for _, ref := range o.GitRefs {
		if _, seenOrg := seen[ref.Org]; seenOrg {
			if seen[ref.Org].Has(ref.Repo) {
				return errors.New("sync config for %s/%s provided more than once")
			}
			seen[ref.Org].Insert(ref.Repo)
		} else {
			seen[ref.Org] = sets.NewString(ref.Repo)
		}
	}

	return nil
}

const (
	// JSONConfigEnvVar is the environment variable that
	// clonerefs expects to find a full JSON configuration
	// in when run.
	JSONConfigEnvVar = "CLONEREFS_OPTIONS"
	// DefaultGitUserName is the default name used in git config
	DefaultGitUserName = "ci-robot"
	// DefaultGitUserEmail is the default email used in git config
	DefaultGitUserEmail = "ci-robot@k8s.io"
)

// ConfigVar exposes the environment variable used
// to store serialized configuration
func (o *Options) ConfigVar() string {
	return JSONConfigEnvVar
}

// LoadConfig loads options from serialized config
func (o *Options) LoadConfig(config string) error {
	return json.Unmarshal([]byte(config), o)
}

// BindOptions binds flags to options
func (o *Options) BindOptions(flags *flag.FlagSet) {
	BindOptions(o, flags)
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.GitRefs = o.refs.gitRefs

	for _, pathAlias := range o.aliases.aliases {
		for _, ref := range o.GitRefs {
			if pathAlias.Org == ref.Org && pathAlias.Repo == ref.Repo {
				ref.PathAlias = pathAlias.Path
			}
		}
	}
}

// BindOptions adds flags to the FlagSet that populate
// the GCS upload options struct given.
func BindOptions(options *Options, fs *flag.FlagSet) {
	fs.StringVar(&options.SrcRoot, "src-root", "", "Where to root source checkouts")
	fs.StringVar(&options.Log, "log", "", "Where to write logs")
	fs.StringVar(&options.GitUserName, "git-user-name", DefaultGitUserName, "Username to set in git config")
	fs.StringVar(&options.GitUserEmail, "git-user-email", DefaultGitUserEmail, "Email to set in git config")
	fs.Var(&options.refs, "repo", "Mapping of Git URI to refs to check out, can be provided more than once")
	fs.Var(&options.aliases, "clone-alias", "Mapping of org and repo to path to clone to, can be provided more than once")
	fs.IntVar(&options.MaxParallelWorkers, "max-workers", 0, "Maximum number of parallel workers, unset for unlimited.")
}

type gitRefs struct {
	gitRefs []*kube.Refs
}

func (r *gitRefs) String() string {
	representation := bytes.Buffer{}
	for _, ref := range r.gitRefs {
		fmt.Fprintf(&representation, "%s,%s=%s", ref.Org, ref.Repo, ref.String())
	}
	return representation.String()
}

// Set parses out a kube.Refs from the user string.
// The following example shows all possible fields:
//   org,repo=base-ref:base-sha[,pull-number:pull-sha]...
// For the base ref and every pull number, the SHAs
// are optional and any number of them may be set or
// unset.
func (r *gitRefs) Set(value string) error {
	gitRef, err := ParseRefs(value)
	if err != nil {
		return err
	}
	r.gitRefs = append(r.gitRefs, gitRef)
	return nil
}

type pathAliases struct {
	aliases []ClonePathAlias
}

func (a *pathAliases) String() string {
	representation := bytes.Buffer{}
	for _, resolver := range a.aliases {
		fmt.Fprint(&representation, resolver.String())
	}
	return representation.String()
}

// Set parses out path aliases from user input
func (a *pathAliases) Set(value string) error {
	resolver, err := ParseAliases(value)
	if err != nil {
		return err
	}
	a.aliases = append(a.aliases, resolver)
	return nil
}

// Encode will encode the set of options in the format that
// is expected for the configuration environment variable
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}
