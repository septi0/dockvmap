package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func (f *Follower) queueCopies() {
	if f.mirror == nil {
		return
	}

	select {
	case f.copies <- struct{}{}:
	default:
	}
}

func (f *Follower) runCopies(ctx context.Context) {
	var retry <-chan time.Time
	delay := followerCopyRetry

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.copies:
		case <-retry:
		}

		retry = nil

		if f.copyRepositories(ctx) == 0 {
			delay = followerCopyRetry
			continue
		}

		retry = time.After(delay)
		delay = min(delay*2, followerMaxCopyRetry)
	}
}

// copyRepositories returns how many repositories failed, to retry later
func (f *Follower) copyRepositories(ctx context.Context) int {
	f.copyMu.Lock()
	defer f.copyMu.Unlock()

	f.mu.Lock()
	latest := f.latest
	f.mu.Unlock()

	if latest == nil || f.isStopped() {
		return 0
	}

	state, err := f.store.FollowerState(ctx)

	if err != nil {
		slog.Error("reading the follower state failed", "error", err)
		return 1
	}

	copied, err := f.store.FollowerRepositories(ctx)

	if err != nil {
		slog.Error("reading copied repositories failed", "error", err)
		return 1
	}

	due, err := f.dueRepositories(ctx, latest.Snapshot.Repositories, copied, state.InitialCopyDone)

	if err != nil {
		slog.Warn("listing what to copy from the primary failed", "error", err)
		return 1
	}

	remaining, failed := len(due), 0
	f.setCopies(remaining, 0)

	for _, repo := range due {
		err := f.copyRepository(ctx, repo, copied[repo.Repository])

		if err == nil {
			err = f.store.SetFollowerRepository(ctx, repo.Repository, repo.Counter)
		}

		if ctx.Err() != nil {
			return 0
		}

		if err != nil {
			failed++
			f.copyFailed(repo.Repository, err)
		} else {
			remaining--
			f.copySucceeded(repo.Repository)
		}

		f.setCopies(remaining, failed)
	}

	// an internal image held back for missing content moves once its tag is copied
	if remaining < len(due) {
		if err := f.applyState(ctx); err != nil {
			slog.Error("applying the state after copying failed", "error", err)
		}
	}

	if failed > 0 {
		return failed
	}

	if err := f.store.MarkInitialCopyDone(ctx); err != nil {
		slog.Error("saving the initial copy failed", "error", err)
	}

	if state.PruneRequested {
		if err := f.removeExtraTags(ctx); err != nil {
			if f.newFailure(pruneFailureKey, err) {
				slog.Warn("removing the images the primary doesn't have failed", "error", err)
			}

			return 1
		}

		f.copySucceeded(pruneFailureKey)
	}

	return 0
}

// dueRepositories lists the repositories whose counter moved; until the first copy is done it adds every other repository on the primary's registry, like images pushed before replication was on
func (f *Follower) dueRepositories(ctx context.Context, repositories []model.ReplicatedRepository, copied map[string]int64, initialCopyDone bool) ([]model.ReplicatedRepository, error) {
	var due []model.ReplicatedRepository
	recorded := make(map[string]bool, len(repositories))

	for _, repo := range repositories {
		recorded[repo.Repository] = true

		if copied[repo.Repository] != repo.Counter {
			due = append(due, repo)
		}
	}

	if initialCopyDone {
		return due, nil
	}

	all, err := f.mirror.Repositories(ctx)

	if err != nil {
		return nil, err
	}

	for _, repository := range all {
		if _, done := copied[repository]; !done && !recorded[repository] {
			due = append(due, model.ReplicatedRepository{Repository: repository})
		}
	}

	return due, nil
}

// copyRepository replays the recorded changes since from, or copies every missing tag when they aren't all recorded
func (f *Follower) copyRepository(ctx context.Context, repo model.ReplicatedRepository, from int64) error {
	if from > 0 {
		changes, err := f.client.Changes(ctx, repo.Repository, from)

		if err != nil {
			return err
		}

		if changes.Complete {
			return f.replay(ctx, repo.Repository, changes.Changes)
		}
	}

	_, err := f.copyTags(ctx, repo.Repository)

	return err
}

func (f *Follower) replay(ctx context.Context, repository string, changes []model.RepositoryChange) error {
	for _, change := range changes {
		if change.Deleted {
			if err := f.mirror.Delete(ctx, repository, change.Reference); err != nil {
				return err
			}

			continue
		}

		if copied, err := f.mirror.Copy(ctx, repository, change.Reference, false); err != nil {
			return err
		} else if copied {
			slog.Info("copied an image from the primary", "repository", repository, "tag", change.Reference)
		}
	}

	return nil
}

func (f *Follower) copyTags(ctx context.Context, repository string) (int, error) {
	tags, err := f.mirror.Tags(ctx, repository)

	if err != nil {
		return 0, err
	}

	copiedTags := 0

	for _, tag := range tags {
		copied, err := f.mirror.Copy(ctx, repository, tag, true)

		if err != nil {
			return copiedTags, err
		}

		if copied {
			copiedTags++
			slog.Info("copied an image from the primary", "repository", repository, "tag", tag)
		}
	}

	return copiedTags, nil
}

// no repository has an empty name
const pruneFailureKey = ""

func (f *Follower) copyFailed(repository string, err error) {
	if f.newFailure(repository, err) {
		slog.Warn("copying from the primary failed", "repository", repository, "error", err)
	}
}

// newFailure reports whether err differs from the last failure under key, so each is logged once until it changes or clears
func (f *Follower) newFailure(key string, err error) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	repeated := f.copyErrors[key] == err.Error()
	f.copyErrors[key] = err.Error()

	return !repeated
}

// copyError is one current copy failure, the same each time; the caller holds f.mu
func (f *Follower) copyError() string {
	for _, key := range slices.Sorted(maps.Keys(f.copyErrors)) {
		if key == pruneFailureKey {
			return "removing the images the primary doesn't have: " + f.copyErrors[key]
		}

		return key + ": " + f.copyErrors[key]
	}

	return ""
}

func (f *Follower) copySucceeded(repository string) {
	f.mu.Lock()
	delete(f.copyErrors, repository)
	f.mu.Unlock()
}

// Reconcile copies what this follower lacks, including missing layers, and reports what the primary lacks without deleting it
func (f *Follower) Reconcile(ctx context.Context) (int, error) {
	if f.mirror == nil {
		return 0, nil
	}

	if f.isStopped() {
		return 0, errors.New("sync from the primary is stopped; see /health/ready")
	}

	f.copyMu.Lock()
	defer f.copyMu.Unlock()

	repositories, err := f.mirror.Repositories(ctx)

	if err != nil {
		return 0, err
	}

	copied, failed := 0, 0

	for _, repository := range repositories {
		n, err := f.copyTags(ctx, repository)
		copied += n

		if err != nil {
			if ctx.Err() != nil {
				return copied, ctx.Err()
			}

			failed++
			f.copyFailed(repository, err)

			continue
		}

		f.copySucceeded(repository)
	}

	if copied > 0 {
		if err := f.applyState(ctx); err != nil {
			return copied, err
		}
	}

	extrasErr := f.reportExtraTags(ctx)

	if failed > 0 {
		return copied, fmt.Errorf("%d of %d repositories failed to copy", failed, len(repositories))
	}

	return copied, extrasErr
}

func (f *Follower) reportExtraTags(ctx context.Context) error {
	extras, err := f.findExtraTags(ctx)

	if err != nil {
		return err
	}

	if names := f.setExtraTags(extras); len(names) > 0 {
		slog.Warn("this follower's registry has images the primary's doesn't; run dockvmap -resync on this follower to remove them", "count", len(names), "images", names)
	}

	return nil
}

// removeExtraTags deletes what the primary's registry lacks, unless it is empty, as after losing its volume
func (f *Follower) removeExtraTags(ctx context.Context) error {
	repositories, err := f.mirror.Repositories(ctx)

	if err != nil {
		return err
	}

	if len(repositories) == 0 {
		slog.Warn("the primary's registry is empty, so this follower keeps its images; once the primary's registry is back, run dockvmap -resync again to remove what it lacks")
		return f.store.ClearPruneRequest(ctx)
	}

	extras, err := f.findExtraTags(ctx)

	if err != nil {
		return err
	}

	names := f.setExtraTags(extras)

	for repository, tags := range extras {
		if err := f.mirror.DeleteTags(ctx, repository, tags); err != nil {
			return err
		}
	}

	if len(names) > 0 {
		slog.Info("removed the images the primary's registry doesn't have", "count", len(names), "images", names)
	}

	f.setExtraTags(nil)

	return f.store.ClearPruneRequest(ctx)
}

func (f *Follower) findExtraTags(ctx context.Context) (map[string][]string, error) {
	local, err := f.mirror.LocalTags(ctx)

	if err != nil {
		return nil, err
	}

	extras := map[string][]string{}

	for repository, tags := range local {
		primary, err := f.mirror.Tags(ctx, repository)

		if err != nil {
			return nil, err
		}

		for _, tag := range tags {
			if !slices.Contains(primary, tag) {
				extras[repository] = append(extras[repository], tag)
			}
		}
	}

	return extras, nil
}

// setExtraTags returns the tags as sorted repository:tag names
func (f *Follower) setExtraTags(extras map[string][]string) []string {
	var names []string

	for repository, tags := range extras {
		for _, tag := range tags {
			names = append(names, repository+":"+tag)
		}
	}

	slices.Sort(names)
	count := len(names)

	f.mu.Lock()
	f.extraTags = &count
	f.mu.Unlock()

	return names
}

func (f *Follower) setCopies(pending, failed int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pending, f.failed = pending, failed
}
