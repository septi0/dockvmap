// Command tagaudit looks for tagging conventions internal/taganalyzer does not yet
// understand, by analysing a large sample of real repositories and reporting the
// shapes that stop tags from grouping.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	corpus := flag.String("corpus", "sampledata/tags", "directory holding one <registry>/<repository>.txt per repo")
	sample := flag.Int("sample", 0, "build a repository list of about this many repos, then exit")
	fetch := flag.Bool("fetch", false, "download tags for every repo in the manifest that is not cached yet")
	shapes := flag.Bool("shapes", false, "report the segment shapes blocking tags from grouping")
	rank := flag.Bool("rank", false, "report repositories whose first family looks wrong")
	verify := flag.Bool("verify", false, "re-check every invariant taganalyzer promises")
	show := flag.String("show", "", "print one repository's families, e.g. docker.io/library/nginx")
	minRepos := flag.Int("min-repos", 3, "a shape must appear in at least this many repos to be reported")
	flag.Parse()

	if err := run(*corpus, *sample, *fetch, *shapes, *rank, *verify, *show, *minRepos); err != nil {
		fmt.Fprintln(os.Stderr, "tagaudit:", err)
		os.Exit(1)
	}
}

func run(corpus string, sample int, fetch, shapes, rank, verify bool, show string, minRepos int) error {
	if sample > 0 {
		repos, err := sampleRepositories(sample)
		if err != nil {
			return err
		}
		return writeManifest(corpus, repos)
	}

	if fetch {
		repos, err := readManifest(corpus)
		if err != nil {
			return err
		}
		return fetchCorpus(corpus, repos)
	}

	analysed, err := analyseCorpus(corpus)
	if err != nil {
		return err
	}
	if len(analysed) == 0 {
		return fmt.Errorf("no repositories found under %s (run -sample then -fetch first)", corpus)
	}
	if show != "" {
		reportRepository(analysed, show)
		return nil
	}

	fmt.Printf("audited %d repositories\n\n", len(analysed))

	all := !shapes && !rank && !verify
	if verify || all {
		reportInvariants(analysed)
	}
	if shapes || all {
		reportShapes(analysed, minRepos)
	}
	if rank || all {
		reportRanking(analysed)
	}
	return nil
}
