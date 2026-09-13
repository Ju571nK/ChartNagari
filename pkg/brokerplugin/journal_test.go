package brokerplugin

import (
	"context"
	"path/filepath"
	"testing"
)

func TestInterruptedReservationSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.db")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, _, err := j.reserve(context.Background(), "submit/account/id", "hash")
	if err != nil || !fresh {
		t.Fatal(err)
	}
	_ = j.Close()
	j, err = OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	fresh, result, conflict, err := j.reserve(context.Background(), "submit/account/id", "hash")
	if err != nil || fresh || result != nil || conflict {
		t.Fatal("in-flight reservation lost after crash")
	}
	_, _, conflict, err = j.reserve(context.Background(), "submit/account/id", "changed")
	if err != nil || !conflict {
		t.Fatal("fingerprint conflict lost")
	}
}

func TestClosedJournalFailsClosed(t *testing.T) {
	j, err := OpenJournal(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	fresh, _, _, err := j.reserve(context.Background(), "id", "hash")
	if err == nil || fresh {
		t.Fatal("closed journal allowed reservation")
	}
}
