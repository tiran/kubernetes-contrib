/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package pulls

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	github_util "k8s.io/contrib/github"
	"k8s.io/contrib/mungegithub/config"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

type PRSizeMunger struct {
	generatedFilesFile string
	genFiles           *sets.String
	genPrefixes        *[]string
}

func init() {
	RegisterMungerOrDie(&PRSizeMunger{})
}

func (PRSizeMunger) Name() string { return "size" }

func (p *PRSizeMunger) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&p.generatedFilesFile, "generated-files-config", "generated-files.txt", "file containing the pathname to label mappings")
}

const labelSizePrefix = "size/"

// getGeneratedFiles returns a list of all automatically generated files in the repo. These include
// docs, deep_copy, and conversions
//
// It would be 'better' to call this for every commit but that takes
// a whole lot of time for almost always the same information, and if
// our results are slightly wrong, who cares? Instead look for the
// generated files once and if someone changed what files are generated
// we'll size slightly wrong. No biggie.
func (s *PRSizeMunger) getGeneratedFiles(config *config.MungeConfig) {
	if s.genFiles != nil {
		return
	}
	files := sets.NewString()
	prefixes := []string{}
	s.genFiles = &files
	s.genPrefixes = &prefixes

	file := s.generatedFilesFile
	if len(file) == 0 {
		glog.Infof("No --generated-files-config= supplied, applying no labels")
		return
	}
	fp, err := os.Open(file)
	if err != nil {
		glog.Errorf("Unable to open %q: %v", file, err)
		return
	}

	defer fp.Close()
	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			glog.Errorf("Invalid line in generated docs config %s: %q", file, line)
			continue
		}
		eType := fields[0]
		file := fields[1]
		if eType == "prefix" {
			prefixes = append(prefixes, file)
		} else if eType == "path" {
			files.Insert(file)
		} else if eType == "paths-from-repo" {
			docs, err := config.GetFileContents(file, "")
			if err != nil {
				continue
			}
			docSlice := strings.Split(docs, "\n")
			files.Insert(docSlice...)
		} else {
			glog.Errorf("Invalid line in generated docs config, unknown type: %s, %q", eType, line)
			continue
		}
	}
	if scanner.Err() != nil {
		glog.Errorf("Error scanning %s: %v", file, err)
		return
	}
	s.genFiles = &files
	s.genPrefixes = &prefixes

	return
}

func (s *PRSizeMunger) MungePullRequest(config *config.MungeConfig, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent) {
	s.getGeneratedFiles(config)
	genFiles := *s.genFiles
	genPrefixes := *s.genPrefixes

	if pr.Additions == nil {
		glog.Warningf("PR %d has nil Additions", *pr.Number)
		return
	}
	adds := *pr.Additions
	if pr.Deletions == nil {
		glog.Warningf("PR %d has nil Deletions", *pr.Number)
		return
	}
	dels := *pr.Deletions

	for _, c := range commits {
		for _, f := range c.Files {
			for _, p := range genPrefixes {
				if strings.HasPrefix(*f.Filename, p) {
					adds = adds - *f.Additions
					dels = dels - *f.Deletions
					continue
				}
			}
			if genFiles.Has(*f.Filename) {
				adds = adds - *f.Additions
				dels = dels - *f.Deletions
				continue
			}
		}
	}

	newSize := calculateSize(adds, dels)
	newLabel := labelSizePrefix + newSize

	existing := github_util.GetLabelsWithPrefix(issue.Labels, labelSizePrefix)
	needsUpdate := true
	for _, l := range existing {
		if l == newLabel {
			needsUpdate = false
			continue
		}
		config.RemoveLabel(*pr.Number, l)
	}
	if needsUpdate {
		config.AddLabels(*pr.Number, []string{newLabel})

		body := fmt.Sprintf("Labelling this PR as %s", newLabel)
		config.WriteComment(*pr.Number, body)
	}
}

const (
	sizeXS  = "XS"
	sizeS   = "S"
	sizeM   = "M"
	sizeL   = "L"
	sizeXL  = "XL"
	sizeXXL = "XXL"
)

func calculateSize(adds, dels int) string {
	lines := adds + dels

	// This is a totally arbitrary heuristic and is open for tweaking.
	if lines < 10 {
		return sizeXS
	}
	if lines < 30 {
		return sizeS
	}
	if lines < 100 {
		return sizeM
	}
	if lines < 500 {
		return sizeL
	}
	if lines < 1000 {
		return sizeXL
	}
	return sizeXXL
}
