package watch

import (
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/devspace-cloud/devspace/pkg/util/fsutil"
	"github.com/devspace-cloud/devspace/pkg/util/log"

	"gotest.tools/assert"
)

type testCase struct {
	name              string
	changes           []testCaseChange
	expectedChanges   []string
	expectedDeletions []string
}

type testCaseChange struct {
	path    string
	content string
	delete  bool
}

func TestWatcher(t *testing.T) {
	t.Skip("Test has a data race")

	watchedPaths := []string{".", "hello.txt", "watchedsubdir"}
	testCases := []testCase{
		{
			name:            "Create text file",
			expectedChanges: []string{".", "hello.txt"},
			changes: []testCaseChange{
				{
					path:    "hello.txt",
					content: "hello",
				},
			},
		},
		{
			name:            "Create file in folder",
			expectedChanges: []string{".", "watchedsubdir"},
			changes: []testCaseChange{
				{
					path:    "watchedsubdir/unwatchedsubfile.txt",
					content: "watchedsubdir",
				},
			},
		},
		{
			name:            "Override file",
			expectedChanges: []string{"hello.txt"},
			changes: []testCaseChange{
				{
					path:    "hello.txt",
					content: "another hello",
				},
			},
		},
		{
			name:              "Delete file",
			expectedChanges:   []string{"."},
			expectedDeletions: []string{"hello.txt"},
			changes: []testCaseChange{
				{
					path:   "hello.txt",
					delete: true,
				},
			},
		},
	}

	// Create TmpFolder
	dir, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatalf("Error creating temporary directory: %v", err)
	}

	wdBackup, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current working directory: %v", err)
	}
	err = os.Chdir(dir)
	if err != nil {
		t.Fatalf("Error changing working directory: %v", err)
	}

	// Cleanup temp folder
	defer os.Chdir(wdBackup)
	defer os.RemoveAll(dir)

	var (
		callbackCalledChan = make(chan bool)
		expectedChanges    = &[]string{}
		expectedDeletions  = &[]string{}
		changeLock         sync.Mutex
	)

	callback := func(changed []string, deleted []string) error {
		changeLock.Lock()
		defer changeLock.Unlock()

		assert.Equal(t, len(*expectedChanges), len(changed), "Wrong changes")
		for index := range changed {
			assert.Equal(t, (*expectedChanges)[index], changed[index], "Wrong changes")
		}

		assert.Equal(t, len(*expectedDeletions), len(deleted), "Wrong deletions")
		for index := range deleted {
			assert.Equal(t, (*expectedDeletions)[index], deleted[index], "Wrong deletions")
		}

		callbackCalledChan <- true
		return nil
	}

	watcher, err := New(watchedPaths, callback, log.GetInstance())
	if err != nil {
		t.Fatalf("Error creating watcher: %v", err)
	}

	watcher.PollInterval = time.Millisecond * 10
	watcher.Start()

	for _, testCase := range testCases {
		changeLock.Lock()
		if testCase.expectedChanges != nil {
			*expectedChanges = testCase.expectedChanges
		} else {
			*expectedChanges = []string{}
		}
		if testCase.expectedDeletions != nil {
			*expectedDeletions = testCase.expectedDeletions
		} else {
			*expectedDeletions = []string{}
		}
		changeLock.Unlock()

		// Apply changes
		for _, change := range testCase.changes {
			if change.delete {
				err = os.RemoveAll(change.path)
				if err != nil {
					t.Fatalf("Error deleting file %s: %v", change.path, err)
				}
			} else {
				err = fsutil.WriteToFile([]byte(change.content), change.path)
				if err != nil {
					t.Fatalf("Error creating file %s: %v", change.path, err)
				}
			}
		}

		select {
		case <-callbackCalledChan:
		case <-time.After(time.Second * 5):
			t.Fatalf("Test %s timed out", testCase.name)
		}
	}

	watcher.Stop()
}