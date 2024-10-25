package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kivattt/gogitstatus"
	"github.com/rivo/tview"
)

type GitStatusHandler struct {
	app             *tview.Application
	channel         chan string
	wg              sync.WaitGroup
	workerWaitGroup sync.WaitGroup
	ctx             context.Context
	cancelFunc      context.CancelFunc

	repoPathCurrentlyWorkingOn string // Does not require a mutex due to workerWaitGroup

	trackedLocalGitRepos      map[string]ChangedFileState
	trackedLocalGitReposMutex sync.Mutex
}

type ChangedFileState struct {
	changedFiles map[string]gogitstatus.ChangedFile
	lastChecked  time.Time
}

// Returns an empty string "" if path is not inside a tracked local git repository
func (gsh *GitStatusHandler) TrackedGitRepositoryContainingPath(path string) string {
	repoFound := ""
	gsh.trackedLocalGitReposMutex.Lock()
	for repoPath := range gsh.trackedLocalGitRepos {
		// TODO: Improve performance? filepath.Rel() seems a little slow
		relativePathToRepo, err := filepath.Rel(repoPath, path)
		if err != nil {
			continue
		}

		if !strings.HasPrefix(relativePathToRepo, "..") { // Hacky
			repoFound = repoPath
			break
		}
	}
	gsh.trackedLocalGitReposMutex.Unlock()

	return repoFound
}

// Looks for the first parent directory of path (or path itself) containing a ".git" directory.
// Returns an empty string ("") if none found.
func (gsh *GitStatusHandler) TryFindContainingGitRepositoryForPath(path string) string {
	repoPathFound := path
	for {
		// Reached root path, no git repository found
		if repoPathFound == filepath.Dir(repoPathFound) {
			return ""
		}

		stat, err := os.Lstat(filepath.Join(repoPathFound, ".git"))
		if err == nil && stat.IsDir() {
			break
		}

		repoPathFound = filepath.Dir(repoPathFound)
	}

	return repoPathFound
}

func (gsh *GitStatusHandler) Init() {
	if gsh.app == nil {
		panic("In GitStatusHandler Init(), app was nil")
	}

	gsh.wg.Add(1)

	// Buffer size of 100 (arbitrary) prevents blocking when scrolling fast
	gsh.channel = make(chan string, 100)

	gsh.trackedLocalGitReposMutex.Lock()
	gsh.trackedLocalGitRepos = make(map[string]ChangedFileState)
	gsh.trackedLocalGitReposMutex.Unlock()

	go func() {
	chanLoop:
		for path := range gsh.channel {
			if !filepath.IsAbs(path) {
				panic("GitStatusHandler received a non-absolute path: \"" + path + "\"")
			}

			repoPathFound := gsh.TryFindContainingGitRepositoryForPath(path)
			if repoPathFound == "" {
				continue chanLoop
			}

			// We don't want to restart a gogitstatus.StatusWithContext() already running on the same path
			if repoPathFound == gsh.repoPathCurrentlyWorkingOn {
				continue chanLoop
			}

			// Remove oldest tracked repository after 15 repos (pretty arbitrary, repos can vary massively in size)
			gsh.trackedLocalGitReposMutex.Lock()
			if len(gsh.trackedLocalGitRepos) > 15 {
				var oldestRepositoryTime time.Time
				oldestRepositoryPath := ""

				for k, v := range gsh.trackedLocalGitRepos {
					if oldestRepositoryPath == "" {
						oldestRepositoryTime = v.lastChecked
						oldestRepositoryPath = k
						continue
					}

					if v.lastChecked.Before(oldestRepositoryTime) {
						oldestRepositoryTime = v.lastChecked
						oldestRepositoryPath = k
					}
				}

				delete(gsh.trackedLocalGitRepos, oldestRepositoryPath)
			}
			gsh.trackedLocalGitReposMutex.Unlock()

			// Cancel the previous gogitstatus.StatusWithContext()
			if gsh.cancelFunc != nil {
				gsh.cancelFunc()
				gsh.workerWaitGroup.Wait()
			}

			gsh.repoPathCurrentlyWorkingOn = repoPathFound

			gsh.ctx, gsh.cancelFunc = context.WithCancel(context.Background())

			go func() {
				defer func() {
					// Allow a git status restart on this path
					gsh.repoPathCurrentlyWorkingOn = ""
					gsh.cancelFunc()
					gsh.workerWaitGroup.Done()
				}()

				gsh.workerWaitGroup.Add(1)

				if !filepath.IsAbs(gsh.repoPathCurrentlyWorkingOn) {
					panic("GitStatusHandler tried to run StatusWithContext on a non-absolute path: \"" + gsh.repoPathCurrentlyWorkingOn + "\"")
				}

				changedFiles, err := gogitstatus.StatusWithContext(gsh.ctx, gsh.repoPathCurrentlyWorkingOn)
				if err != nil {
					return
				}

				gsh.trackedLocalGitReposMutex.Lock()
				gsh.trackedLocalGitRepos[gsh.repoPathCurrentlyWorkingOn] = ChangedFileState{
					changedFiles: changedFiles,
					lastChecked:  time.Now(),
				}
				gsh.trackedLocalGitReposMutex.Unlock()

				gsh.app.QueueUpdateDraw(func() {})
			}()
		}
		gsh.wg.Done()
	}()
}
