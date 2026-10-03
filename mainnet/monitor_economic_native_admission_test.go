//go:build linux

// The largest accepted history must be read before the first public sample.
// Real access events expose progress without skipping hash or custody checks;
// the public sample still proves that all admission and chain checks finished.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type monitorNativeAdmissionObservation struct {
	count int
	err   error
}

// The reader has one owner and joins on every return. A 300s bound is only a
// liveness backstop; readiness is the positive access census, never elapsed time.
func monitorNativeObserveAdmission(t *testing.T, references []monitorHistoryReference) func(*monitorEconomicTestRun) {
	t.Helper()
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(fd), "synthetic-native-admission-events")
	watchNames := map[int]string{}
	for _, reference := range references {
		watch, err := unix.InotifyAddWatch(fd, reference.Path, unix.IN_ACCESS|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_ATTRIB)
		if err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if _, present := watchNames[watch]; present {
			_ = file.Close()
			t.Fatal("archive admission watches alias one inode")
		}
		watchNames[watch] = reference.Path
	}
	observations := make(chan monitorNativeAdmissionObservation, len(references)+1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		seen := map[int]bool{}
		buffer := make([]byte, 64*1024)
		for {
			n, err := file.Read(buffer)
			if err != nil {
				observations <- monitorNativeAdmissionObservation{count: len(seen), err: err}
				return
			}
			for offset := 0; offset < n; {
				if n-offset < unix.SizeofInotifyEvent {
					observations <- monitorNativeAdmissionObservation{count: len(seen), err: errors.New("partial inotify event")}
					return
				}
				watch := int(int32(binary.NativeEndian.Uint32(buffer[offset : offset+4])))
				mask := binary.NativeEndian.Uint32(buffer[offset+4 : offset+8])
				size := int(binary.NativeEndian.Uint32(buffer[offset+12 : offset+16]))
				if size > n-offset-unix.SizeofInotifyEvent {
					observations <- monitorNativeAdmissionObservation{count: len(seen), err: errors.New("oversized inotify event")}
					return
				}
				offset += unix.SizeofInotifyEvent + size
				if mask&(unix.IN_Q_OVERFLOW|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_IGNORED) != 0 {
					observations <- monitorNativeAdmissionObservation{count: len(seen), err: fmt.Errorf("archive custody access census invalidated: %x", mask)}
					return
				}
				if _, present := watchNames[watch]; !present {
					observations <- monitorNativeAdmissionObservation{count: len(seen), err: errors.New("unknown archive access watch")}
					return
				}
				if mask&unix.IN_ACCESS != 0 && !seen[watch] {
					seen[watch] = true
					observations <- monitorNativeAdmissionObservation{count: len(seen)}
					if len(seen) == len(references) {
						return
					}
				}
			}
		}
	}()
	closed := false
	closeObserver := func() {
		if closed {
			return
		}
		closed = true
		_ = file.Close()
		<-joined
	}
	t.Cleanup(closeObserver)
	return func(run *monitorEconomicTestRun) {
		t.Helper()
		defer closeObserver()
		started := time.Now()
		timer := time.NewTimer(300 * time.Second)
		defer timer.Stop()
		count := 0
		for count < len(references) {
			select {
			case observation := <-observations:
				count = observation.count
				if observation.err != nil {
					t.Fatal("actual archive admission read failed", count, len(references), observation.err)
				}
			case <-run.done:
				t.Fatal("public native worker returned before complete archive admission", count, len(references), run.exit, run.diagnostic.String())
			case <-t.Context().Done():
				t.Fatal("caller canceled archive admission", count, len(references), t.Context().Err())
			case <-timer.C:
				t.Fatal("archive admission progress budget exhausted", count, len(references))
			}
		}
		t.Logf("actual native startup read all %d retained segments in %s before sample assertion", count, time.Since(started))
	}
}
