package syncx

import (
	"context"
	"testing"
	"time"
)

func Test_WaitGroup_Wait(t *testing.T) {
	wg := NewWaitGroup(context.Background())

	err := wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}
}

func Test_WaitGroup_Done(t *testing.T) {
	wg := NewWaitGroup(context.Background())

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			wg.Done()
		})
	}()

	err := wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}
}

func Test_WaitGroup_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wg := NewWaitGroup(ctx)

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			cancel()
		})
	}()

	err := wg.Wait()
	if err != ExceededMaxTime {
		t.Errorf("Wanted err: %v. Got: %v", ExceededMaxTime, err)
	}

	wg = NewWaitGroup(context.Background())

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			wg.Cancel()
		})
	}()

	err = wg.Wait()
	if err != Cancelled {
		t.Errorf("Wanted err: %v. Got: %v", Cancelled, err)
	}
}

func Test_WaitGroup_After_Done(t *testing.T) {
	wg := NewWaitGroup(context.Background())

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			wg.Done()
		})
	}()
	err := wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}

	// Test that nothing happens if a WaitGroup which is done, is manipulated
	wg.Add(1)
	wg.Done()
	err = wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}
}
